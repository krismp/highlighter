package combine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/ffmpeg"
)

var (
	ErrBusy         = errors.New("videos are already being combined; wait until that finishes")
	ErrShuttingDown = errors.New("highlighter is shutting down")
)

// Runner runs ffmpeg: Run for an encode with -progress output, Concat to
// join files by stream copy.
type Runner interface {
	Run(ctx context.Context, args []string, onLine func(string)) error
	Concat(ctx context.Context, inputs []string, output string) error
}

type Status string

const (
	Idle       Status = ""
	Processing Status = "processing"
	Done       Status = "done"
	Failed     Status = "failed"
	Canceled   Status = "canceled"
)

// View is the state of the latest combine.
type View struct {
	Status    Status    `json:"status"`
	Running   bool      `json:"running"`
	Progress  float64   `json:"progress"` // 0..1
	Copy      bool      `json:"copy"`     // joined without re-encoding
	Count     int       `json:"count"`    // videos joined
	Dir       string    `json:"dir"`
	File      string    `json:"file"` // output file name, in Dir
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	Duration  float64   `json:"duration"` // seconds
	Size      int64     `json:"size"`     // bytes, once done
	Error     string    `json:"error"`
	StartedAt time.Time `json:"startedAt"`
	Trashed   bool      `json:"trashed"` // the result was moved to the Trash
}

// Path is the output file's full path, or "" before a combine.
func (v View) Path() string {
	if v.File == "" {
		return ""
	}
	return filepath.Join(v.Dir, v.File)
}

type Request struct {
	Inputs     []Input
	Size       Size
	Transition Transition
	Encoder    ffmpeg.Encoder
	Dir        string // output folder, created when missing
	Name       string // output file name without .mp4
}

// Manager runs one combine at a time and fans its state out to subscribers.
type Manager struct {
	runner Runner

	mu     sync.Mutex
	view   View
	cancel context.CancelFunc

	subsMu sync.Mutex
	subs   map[chan View]struct{}

	ctx     context.Context // cancelled by Shutdown
	stop    context.CancelFunc
	workers sync.WaitGroup
}

func NewManager(r Runner) *Manager {
	ctx, stop := context.WithCancel(context.Background())
	return &Manager{runner: r, subs: map[chan View]struct{}{}, ctx: ctx, stop: stop}
}

func (m *Manager) View() View {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.view
}

// Start begins combining req.Inputs, in order, into a new file in req.Dir;
// an existing file of that name gets " 2", " 3"… added, never overwritten.
func (m *Manager) Start(req Request) (View, error) {
	if m.ctx.Err() != nil {
		return View{}, ErrShuttingDown
	}
	if len(req.Inputs) < 2 {
		return View{}, errors.New("add at least two videos to combine")
	}
	if err := os.MkdirAll(req.Dir, 0o755); err != nil {
		return View{}, err
	}
	m.mu.Lock()
	if m.view.Running {
		m.mu.Unlock()
		return View{}, ErrBusy
	}
	plan := NewPlan(req.Inputs, req.Size, req.Transition)
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.view = View{
		Status: Processing, Running: true, Copy: plan.Copy, Count: len(req.Inputs),
		Dir: req.Dir, File: uniqueName(req.Dir, req.Name, ".mp4"),
		Width: plan.Width, Height: plan.Height, Duration: plan.Total, StartedAt: time.Now().UTC(),
	}
	v := m.view
	m.mu.Unlock()
	m.broadcast(v)

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		defer cancel()
		err := m.render(ctx, req, plan, v.Path())
		size := int64(0)
		if fi, statErr := os.Stat(v.Path()); err == nil && statErr == nil {
			size = fi.Size()
		}
		m.update(func(v *View) {
			v.Running = false
			switch {
			case err == nil:
				v.Status, v.Progress, v.Size = Done, 1, size
			case ctx.Err() != nil:
				v.Status = Canceled
			default:
				v.Status, v.Error = Failed, err.Error()
			}
		})
	}()
	return v, nil
}

func (m *Manager) render(ctx context.Context, req Request, plan Plan, output string) error {
	if plan.Copy {
		paths := make([]string, len(req.Inputs))
		for i, in := range req.Inputs {
			paths[i] = in.Path
		}
		return m.runner.Concat(ctx, paths, output)
	}
	filter, err := os.CreateTemp("", "highlighter-combine-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(filter.Name())
	if _, err := filter.WriteString(Filter(req.Inputs, plan, req.Transition)); err != nil {
		filter.Close()
		return err
	}
	if err := filter.Close(); err != nil {
		return err
	}
	part := output + ".part"
	defer os.Remove(part)
	err = m.runner.Run(ctx, EncodeArgs(req.Inputs, filter.Name(), req.Encoder, part), func(line string) {
		if p, ok := ffmpeg.ProgressFraction(line, plan.Total); ok {
			m.update(func(v *View) { v.Progress = p })
		}
	})
	if err != nil {
		return err
	}
	return os.Rename(part, output)
}

// Cancel stops the running combine; its partial file is removed.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.view.Running && m.cancel != nil {
		m.cancel()
	}
}

// Reset forgets a finished combine's result (the file stays). It reports
// false while a combine is running.
func (m *Manager) Reset() bool {
	m.mu.Lock()
	if m.view.Running {
		m.mu.Unlock()
		return false
	}
	m.view = View{}
	v := m.view
	m.mu.Unlock()
	m.broadcast(v)
	return true
}

// MarkTrashed records that the finished result was moved to the Trash.
func (m *Manager) MarkTrashed() View {
	return m.update(func(v *View) { v.Trashed, v.Size = true, 0 })
}

// Shutdown stops a running combine and waits up to timeout for it.
func (m *Manager) Shutdown(timeout time.Duration) bool {
	m.stop()
	done := make(chan struct{})
	go func() {
		m.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Subscribe returns the current state and a channel of every later change;
// slow subscribers only miss intermediate states.
func (m *Manager) Subscribe() (View, <-chan View, func()) {
	ch := make(chan View, 1)
	m.mu.Lock()
	m.subsMu.Lock()
	m.subs[ch] = struct{}{}
	m.subsMu.Unlock()
	v := m.view
	m.mu.Unlock()
	return v, ch, func() {
		m.subsMu.Lock()
		delete(m.subs, ch)
		m.subsMu.Unlock()
	}
}

func (m *Manager) update(fn func(*View)) View {
	m.mu.Lock()
	fn(&m.view)
	v := m.view
	m.mu.Unlock()
	m.broadcast(v)
	return v
}

func (m *Manager) broadcast(v View) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	for ch := range m.subs {
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
}

// uniqueName is stem+ext, or "stem 2"+ext, "stem 3"+ext… when that name (or
// its partial file) is already in dir.
func uniqueName(dir, stem, ext string) string {
	for i := 1; ; i++ {
		name := stem + ext
		if i > 1 {
			name = fmt.Sprintf("%s %d%s", stem, i, ext)
		}
		if !exists(filepath.Join(dir, name)) && !exists(filepath.Join(dir, name+".part")) {
			return name
		}
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
