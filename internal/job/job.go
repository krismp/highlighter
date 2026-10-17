// Package job runs clip-cutting jobs: a worker pool per session, automatic
// retries with re-resolved stream URLs, the optional combined reel, and
// latest-state fan-out to subscribers.
//
// Lock order: Manager.mu → job.mu → Manager.subsMu. Never take a lock to the
// left while holding one to the right.
package job

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/ytdlp"
)

var (
	ErrBusy         = errors.New("a job is already running for this video")
	ErrNotFound     = errors.New("no such job or clip")
	ErrNotRetryable = errors.New("only failed items can be retried")
)

const (
	ConcatID   = "concat"
	ConcatFile = "highlights_full.mp4" // reel file name when Request.ReelFile is empty
	PartsDir   = ".parts"              // reel-only clips are cut here, then removed
)

// Mode says which files a job leaves in the output folder.
type Mode string

const (
	ModeClips Mode = "clips" // one file per clip (also the zero value's behaviour)
	ModeReel  Mode = "reel"  // only the combined reel
	ModeBoth  Mode = "both"  // every clip and the combined reel
)

func (m Mode) Valid() bool { return m == ModeClips || m == ModeReel || m == ModeBoth }

type Resolver interface {
	Resolve(ctx context.Context, url string) (ytdlp.Info, error)
}

type Cutter interface {
	Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error
	Concat(ctx context.Context, inputs []string, output string) error
}

type Status string

const (
	Queued     Status = "queued"
	Processing Status = "processing"
	Done       Status = "done"
	Failed     Status = "failed"
)

type ClipState struct {
	ClipID   string  `json:"clipId"`
	File     string  `json:"file"`
	Status   Status  `json:"status"`
	Progress float64 `json:"progress"`
	Attempt  int     `json:"attempt"`
	Error    string  `json:"error"`
	Duration float64 `json:"duration"` // seconds of video in the file
	Size     int64   `json:"size"`     // bytes, set once done
}

type ConcatState struct {
	Requested bool    `json:"requested"`
	Status    Status  `json:"status"`
	File      string  `json:"file"`
	Error     string  `json:"error"`
	Duration  float64 `json:"duration"`
	Size      int64   `json:"size"`
}

type View struct {
	SessionID string      `json:"sessionId"`
	OutputDir string      `json:"outputDir"`
	Mode      Mode        `json:"mode"`
	Clips     []ClipState `json:"clips"`
	Concat    ConcatState `json:"concat"`
	Running   bool        `json:"running"`
}

type Task struct {
	ClipID string
	Range  clip.Range
	File   string
}

type Request struct {
	SessionID   string
	URL         string
	OutputDir   string
	Tasks       []Task
	Mode        Mode
	ReelFile    string // combined reel file name; ConcatFile when empty
	Encoder     ffmpeg.Encoder
	Concurrency int
}

type Manager struct {
	// Backoff before each automatic retry; its length is the retry count.
	Backoff []time.Duration

	resolver Resolver
	cutter   Cutter

	mu   sync.Mutex
	jobs map[string]*job

	subsMu sync.Mutex
	subs   map[string]map[chan View]struct{}
}

func NewManager(r Resolver, c Cutter) *Manager {
	return &Manager{
		Backoff:  []time.Duration{2 * time.Second, 5 * time.Second},
		resolver: r,
		cutter:   c,
		jobs:     map[string]*job{},
		subs:     map[string]map[chan View]struct{}{},
	}
}

type job struct {
	req Request
	sem chan struct{}

	mu     sync.Mutex
	clips  []ClipState
	concat ConcatState
	active int // clips queued or processing

	resolveMu sync.Mutex
	info      *ytdlp.Info
}

func (j *job) viewLocked() View {
	return View{
		SessionID: j.req.SessionID,
		OutputDir: j.req.OutputDir,
		Mode:      j.req.Mode,
		Clips:     slices.Clone(j.clips),
		Concat:    j.concat,
		Running:   j.active > 0 || j.concat.Status == Processing,
	}
}

func (m *Manager) Start(req Request) (View, error) {
	req.Concurrency = max(req.Concurrency, 1)
	if req.Mode == "" {
		req.Mode = ModeClips
	}
	if req.ReelFile == "" {
		req.ReelFile = ConcatFile
	}
	if req.Mode == ModeReel {
		if err := os.MkdirAll(filepath.Join(req.OutputDir, PartsDir), 0o755); err != nil {
			return View{}, err
		}
	}
	m.mu.Lock()
	if old, ok := m.jobs[req.SessionID]; ok {
		old.mu.Lock()
		running := old.viewLocked().Running
		old.mu.Unlock()
		if running {
			m.mu.Unlock()
			return View{}, ErrBusy
		}
	}
	j := &job{req: req, sem: make(chan struct{}, req.Concurrency), clips: make([]ClipState, len(req.Tasks))}
	var total float64
	for i, t := range req.Tasks {
		j.clips[i] = ClipState{ClipID: t.ClipID, File: t.File, Status: Queued, Duration: t.Range.Duration}
		total += t.Range.Duration
	}
	if req.Mode != ModeClips {
		j.concat = ConcatState{Requested: true, Status: Queued, File: req.ReelFile, Duration: total}
	}
	j.active = len(req.Tasks)
	m.jobs[req.SessionID] = j
	m.mu.Unlock()

	v := m.update(j, func() {})
	for i := range req.Tasks {
		go m.runClip(j, i, false)
	}
	return v, nil
}

func (m *Manager) Retry(sessionID, clipID string) (View, error) {
	m.mu.Lock()
	// Hold m.mu across the state change so Start cannot replace the job
	// between lookup and update (which would bypass ErrBusy).
	j, ok := m.jobs[sessionID]
	if !ok {
		m.mu.Unlock()
		return View{}, ErrNotFound
	}
	if clipID == ConcatID {
		retryable := false
		v := m.update(j, func() {
			if j.concat.Status == Failed {
				j.concat.Status, j.concat.Error = Processing, ""
				retryable = true
			}
		})
		m.mu.Unlock()
		if !retryable {
			return v, ErrNotRetryable
		}
		go m.runConcat(j)
		return v, nil
	}
	idx, retryable := -1, false
	v := m.update(j, func() {
		for i, c := range j.clips {
			if c.ClipID != clipID {
				continue
			}
			idx = i
			if c.Status == Failed {
				j.clips[i] = ClipState{ClipID: c.ClipID, File: c.File, Status: Queued, Duration: c.Duration}
				j.active++
				retryable = true
			}
		}
	})
	m.mu.Unlock()
	if idx < 0 {
		return v, ErrNotFound
	}
	if !retryable {
		return v, ErrNotRetryable
	}
	go m.runClip(j, idx, true)
	return v, nil
}

func (m *Manager) View(sessionID string) (View, bool) {
	m.mu.Lock()
	j, ok := m.jobs[sessionID]
	m.mu.Unlock()
	if !ok {
		return View{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.viewLocked(), true
}

// Subscribe registers for every View change of the session's current and
// future jobs. Slow subscribers only ever miss intermediate states.
func (m *Manager) Subscribe(sessionID string) (*View, <-chan View, func()) {
	ch := make(chan View, 1)
	add := func() {
		m.subsMu.Lock()
		if m.subs[sessionID] == nil {
			m.subs[sessionID] = map[chan View]struct{}{}
		}
		m.subs[sessionID][ch] = struct{}{}
		m.subsMu.Unlock()
	}
	m.mu.Lock()
	j := m.jobs[sessionID]
	m.mu.Unlock()
	var current *View
	if j != nil {
		j.mu.Lock()
		add()
		v := j.viewLocked()
		current = &v
		j.mu.Unlock()
	} else {
		add()
	}
	cancel := func() {
		m.subsMu.Lock()
		delete(m.subs[sessionID], ch)
		m.subsMu.Unlock()
	}
	return current, ch, cancel
}

// update mutates job state under its lock and broadcasts the new view.
func (m *Manager) update(j *job, fn func()) View {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn()
	v := j.viewLocked()
	m.subsMu.Lock()
	for ch := range m.subs[j.req.SessionID] {
		select { // keep only the latest view in the 1-slot buffer
		case ch <- v:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- v:
			default:
			}
		}
	}
	m.subsMu.Unlock()
	return v
}

// runClip cuts one clip; refreshFirst forces stream URLs to be re-resolved on
// the first attempt (manual retries, whose cached URLs may have expired).
func (m *Manager) runClip(j *job, i int, refreshFirst bool) {
	j.sem <- struct{}{}
	defer func() { <-j.sem }()
	var err error
	for attempt := 0; attempt <= len(m.Backoff); attempt++ {
		if attempt > 0 {
			time.Sleep(m.Backoff[attempt-1])
		}
		m.update(j, func() {
			c := &j.clips[i]
			c.Status, c.Attempt, c.Progress, c.Error = Processing, attempt+1, 0, ""
		})
		if err = m.cutOnce(j, i, attempt > 0 || refreshFirst); err == nil {
			break
		}
	}
	startConcat := false
	size := fileSize(j.clipPath(i))
	m.update(j, func() {
		c := &j.clips[i]
		if err != nil {
			c.Status, c.Error = Failed, err.Error()
		} else {
			c.Status, c.Progress, c.Size = Done, 1, size
		}
		j.active--
		if j.concat.Requested && j.concat.Status == Queued && allDone(j.clips) {
			j.concat.Status = Processing
			startConcat = true
		}
	})
	if startConcat {
		m.runConcat(j)
	}
}

func allDone(clips []ClipState) bool {
	for _, c := range clips {
		if c.Status != Done {
			return false
		}
	}
	return true
}

func (m *Manager) cutOnce(j *job, i int, refresh bool) error {
	ctx := context.Background()
	info, err := m.streams(ctx, j, refresh)
	if err != nil {
		return err
	}
	t := j.req.Tasks[i]
	spec := ffmpeg.CutSpec{
		Video: info.Video, Audio: info.Audio,
		Start: t.Range.Start, Duration: t.Range.Duration,
		Encoder: j.req.Encoder,
		Output:  j.clipPath(i),
	}
	return m.cutter.Cut(ctx, spec, func(p float64) {
		m.update(j, func() { j.clips[i].Progress = p })
	})
}

// streams resolves stream URLs once per job, and again on every retry
// because YouTube stream URLs expire and can start returning 403.
func (m *Manager) streams(ctx context.Context, j *job, refresh bool) (ytdlp.Info, error) {
	j.resolveMu.Lock()
	defer j.resolveMu.Unlock()
	if j.info == nil || refresh {
		info, err := m.resolver.Resolve(ctx, j.req.URL)
		if err != nil {
			return ytdlp.Info{}, err
		}
		j.info = &info
	}
	return *j.info, nil
}

func (m *Manager) runConcat(j *job) {
	inputs := make([]string, len(j.req.Tasks))
	for i := range j.req.Tasks {
		inputs[i] = j.clipPath(i)
	}
	output := filepath.Join(j.req.OutputDir, j.req.ReelFile)
	err := m.cutter.Concat(context.Background(), inputs, output)
	if err == nil && j.req.Mode == ModeReel {
		// The parts only existed to build the reel. Kept on failure so a retry can reuse them.
		os.RemoveAll(filepath.Join(j.req.OutputDir, PartsDir))
	}
	size := fileSize(output)
	m.update(j, func() {
		if err != nil {
			j.concat.Status, j.concat.Error = Failed, err.Error()
		} else {
			j.concat.Status, j.concat.Size = Done, size
		}
	})
}

// clipPath is where clip i is written: the output folder, or its parts
// folder when only the reel is kept.
func (j *job) clipPath(i int) string {
	dir := j.req.OutputDir
	if j.req.Mode == ModeReel {
		dir = filepath.Join(dir, PartsDir)
	}
	return filepath.Join(dir, j.req.Tasks[i].File)
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
