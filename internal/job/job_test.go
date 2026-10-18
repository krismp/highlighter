package job

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/ytdlp"
)

type fakeResolver struct {
	calls atomic.Int32
	err   error
}

func (f *fakeResolver) Resolve(ctx context.Context, url string) (ytdlp.Info, error) {
	f.calls.Add(1)
	if f.err != nil {
		return ytdlp.Info{}, f.err
	}
	return ytdlp.Info{ID: "abc", Video: ytdlp.Stream{URL: "https://v"}, Audio: &ytdlp.Stream{URL: "https://a"}}, nil
}

type fakeCutter struct {
	mu        sync.Mutex
	failures  map[string]int // output basename -> failures remaining
	cuts      []ffmpeg.CutSpec
	concats   [][]string
	delay     time.Duration
	gate      chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
}

func (f *fakeCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for m := f.maxActive.Load(); n > m && !f.maxActive.CompareAndSwap(m, n); m = f.maxActive.Load() {
	}
	if f.gate != nil {
		<-f.gate
	}
	time.Sleep(f.delay)
	onProgress(0.5)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cuts = append(f.cuts, spec)
	if name := filepath.Base(spec.Output); f.failures[name] > 0 {
		f.failures[name]--
		return errors.New("boom")
	}
	return nil
}

func (f *fakeCutter) Concat(ctx context.Context, inputs []string, output string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.concats = append(f.concats, append([]string{output}, inputs...))
	return nil
}

func newManager(c *fakeCutter, r *fakeResolver) *Manager {
	m := NewManager(r, c)
	m.Backoff = []time.Duration{0, 0}
	return m
}

func request(n int, mode Mode) Request {
	req := Request{SessionID: "abc", URL: "https://www.youtube.com/watch?v=abc", OutputDir: "/out",
		Mode: mode, Encoder: ffmpeg.X264, Concurrency: 3}
	for i := range n {
		req.Tasks = append(req.Tasks, Task{
			ClipID: string(rune('a' + i)),
			Range:  clip.Range{Start: float64(10 * i), Duration: 10},
			File:   string(rune('a'+i)) + ".mp4",
		})
	}
	return req
}

func waitFor(t *testing.T, m *Manager, cond func(View) bool) View {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := m.View("abc"); ok && cond(v) {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	v, _ := m.View("abc")
	t.Fatalf("condition not met; last view %+v", v)
	return v
}

func idle(v View) bool { return !v.Running }

func TestRunsAllClips(t *testing.T) {
	c := &fakeCutter{}
	m := newManager(c, &fakeResolver{})
	if _, err := m.Start(request(2, ModeClips)); err != nil {
		t.Fatal(err)
	}
	v := waitFor(t, m, idle)
	for _, s := range v.Clips {
		if s.Status != Done || s.Progress != 1 || s.Attempt != 1 {
			t.Errorf("clip state %+v", s)
		}
	}
	if v.Concat.Status != "" || len(c.concats) != 0 {
		t.Errorf("concat should not run: %+v", v.Concat)
	}
	slices.SortFunc(c.cuts, func(a, b ffmpeg.CutSpec) int { return int(a.Start - b.Start) })
	got := c.cuts[1]
	if got.Start != 10 || got.Duration != 10 || got.Output != filepath.FromSlash("/out/b.mp4") || got.Encoder != ffmpeg.X264 ||
		got.Video.URL != "https://v" || got.Audio.URL != "https://a" {
		t.Errorf("cut spec %+v", got)
	}
}

func TestRetriesTransientFailuresAndReResolves(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"a.mp4": 2}}
	r := &fakeResolver{}
	m := newManager(c, r)
	m.Start(request(1, ModeClips))
	v := waitFor(t, m, idle)
	if s := v.Clips[0]; s.Status != Done || s.Attempt != 3 {
		t.Errorf("state %+v; want done on attempt 3", s)
	}
	if r.calls.Load() != 3 {
		t.Errorf("resolver calls = %d; want 3 (initial + one per retry)", r.calls.Load())
	}
}

func TestGivesUpAfterTwoRetriesThenManualRetry(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"b.mp4": 3}}
	r := &fakeResolver{}
	m := newManager(c, r)
	m.Start(request(2, ModeClips))
	v := waitFor(t, m, idle)
	if s := v.Clips[1]; s.Status != Failed || s.Error != "boom" || s.Attempt != 3 {
		t.Fatalf("state %+v; want failed after 3 attempts", s)
	}
	if v.Clips[0].Status != Done {
		t.Errorf("other clip must keep going: %+v", v.Clips[0])
	}
	before := r.calls.Load()
	if _, err := m.Retry("abc", "b"); err != nil {
		t.Fatal(err)
	}
	v = waitFor(t, m, func(v View) bool { return v.Clips[1].Status == Done })
	if got := r.calls.Load(); got != before+1 {
		t.Errorf("resolver calls after manual retry = %d; want %d (re-resolve on first attempt)", got, before+1)
	}
	if v.Running {
		t.Error("job should be idle after retry")
	}
}

func TestRetryErrors(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{})
	if _, err := m.Retry("abc", "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no job: err = %v", err)
	}
	m.Start(request(1, ModeClips))
	waitFor(t, m, idle)
	if _, err := m.Retry("abc", "a"); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("done clip: err = %v", err)
	}
	if _, err := m.Retry("abc", "zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown clip: err = %v", err)
	}
}

func TestResolveFailureFailsClip(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{err: errors.New("yt-dlp: ERROR: Private video")})
	m.Start(request(1, ModeClips))
	v := waitFor(t, m, idle)
	if s := v.Clips[0]; s.Status != Failed || s.Error != "yt-dlp: ERROR: Private video" {
		t.Errorf("state %+v", s)
	}
}

func TestRespectsConcurrency(t *testing.T) {
	c := &fakeCutter{delay: 20 * time.Millisecond}
	m := newManager(c, &fakeResolver{})
	req := request(6, ModeClips)
	req.Concurrency = 2
	m.Start(req)
	waitFor(t, m, idle)
	if got := c.maxActive.Load(); got > 2 {
		t.Errorf("max concurrent cuts = %d; want <= 2", got)
	}
}

func TestConcatAfterAllDoneInOrder(t *testing.T) {
	c := &fakeCutter{}
	m := newManager(c, &fakeResolver{})
	m.Start(request(3, ModeBoth))
	v := waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
	want := []string{"/out/highlights_full.mp4", "/out/a.mp4", "/out/b.mp4", "/out/c.mp4"}
	for i := range want {
		want[i] = filepath.FromSlash(want[i])
	}
	if len(c.concats) != 1 || !slices.Equal(c.concats[0], want) {
		t.Errorf("concats = %v; want [%v]", c.concats, want)
	}
	if v.Concat.File != ConcatFile || v.Running {
		t.Errorf("concat state %+v running=%v", v.Concat, v.Running)
	}
}

func TestConcatWaitsForManualRetry(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"b.mp4": 3}}
	m := newManager(c, &fakeResolver{})
	m.Start(request(2, ModeBoth))
	v := waitFor(t, m, idle)
	if v.Concat.Status != Queued || len(c.concats) != 0 {
		t.Fatalf("concat must wait: %+v", v.Concat)
	}
	m.Retry("abc", "b")
	waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
}

func TestStartWhileRunningIsBusy(t *testing.T) {
	c := &fakeCutter{gate: make(chan struct{})}
	m := newManager(c, &fakeResolver{})
	m.Start(request(1, ModeClips))
	if _, err := m.Start(request(1, ModeClips)); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v; want ErrBusy", err)
	}
	close(c.gate)
	waitFor(t, m, idle)
	if _, err := m.Start(request(1, ModeClips)); err != nil {
		t.Errorf("restart after idle: %v", err)
	}
}

func TestSubscribeReceivesUpdates(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{})
	current, updates, cancel := m.Subscribe("abc")
	defer cancel()
	if current != nil {
		t.Fatalf("no job yet, current = %+v", current)
	}
	m.Start(request(2, ModeClips))
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v := <-updates:
			if !v.Running && v.Clips[0].Status == Done && v.Clips[1].Status == Done {
				return
			}
		case <-timeout:
			t.Fatal("never received the final view")
		}
	}
}

// failThenGate fails every cut until fail is cleared, then blocks on gate.
type failThenGate struct {
	fail atomic.Bool
	gate chan struct{}
}

func (f *failThenGate) Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error {
	if f.fail.Load() {
		return errors.New("boom")
	}
	<-f.gate
	return nil
}

func (f *failThenGate) Concat(ctx context.Context, inputs []string, output string) error { return nil }

func TestRetryAndStartNeverBothSucceed(t *testing.T) {
	for range 50 {
		c := &failThenGate{gate: make(chan struct{})}
		c.fail.Store(true)
		m := NewManager(&fakeResolver{}, c)
		m.Backoff = nil
		m.Start(request(1, ModeClips))
		waitFor(t, m, idle)
		c.fail.Store(false)

		var wg sync.WaitGroup
		var retryErr, startErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, retryErr = m.Retry("abc", "a") }()
		go func() { defer wg.Done(); _, startErr = m.Start(request(1, ModeClips)) }()
		wg.Wait()
		if retryErr == nil && startErr == nil {
			t.Fatal("Retry and Start both succeeded: two jobs running for one session")
		}
		close(c.gate)
		waitFor(t, m, idle)
	}
}

// writingCutter really writes its outputs so file placement and cleanup can be checked.
type writingCutter struct {
	mu      sync.Mutex
	outputs []string
	concats [][]string
}

func (w *writingCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error {
	w.mu.Lock()
	w.outputs = append(w.outputs, spec.Output)
	w.mu.Unlock()
	return os.WriteFile(spec.Output, []byte("clip-bytes"), 0o644)
}

func (w *writingCutter) Concat(ctx context.Context, inputs []string, output string) error {
	w.mu.Lock()
	w.concats = append(w.concats, append([]string{output}, inputs...))
	w.mu.Unlock()
	return os.WriteFile(output, []byte("reel-bytes!"), 0o644)
}

func TestReelOnlyCutsIntoPartsAndRemovesThem(t *testing.T) {
	out := t.TempDir()
	c := &writingCutter{}
	m := newManager(&fakeCutter{}, &fakeResolver{})
	m.cutter = c
	req := request(3, ModeReel)
	req.OutputDir, req.ReelFile = out, "my_reel.mp4"
	if _, err := m.Start(req); err != nil {
		t.Fatal(err)
	}
	v := waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
	parts := filepath.Join(out, PartsDir)
	for _, o := range c.outputs {
		if filepath.Dir(o) != parts {
			t.Errorf("reel-only clip written to %s; want inside %s", o, parts)
		}
	}
	if got := c.concats[0]; got[0] != filepath.Join(out, "my_reel.mp4") || filepath.Dir(got[1]) != parts {
		t.Errorf("concat = %v", got)
	}
	if _, err := os.Stat(parts); !os.IsNotExist(err) {
		t.Errorf("parts dir should be removed after the reel is built, stat err = %v", err)
	}
	if v.Mode != ModeReel || v.Concat.File != "my_reel.mp4" || v.Concat.Duration != 30 || v.Concat.Size != int64(len("reel-bytes!")) {
		t.Errorf("view %+v", v)
	}
}

func TestDoneClipsCarryDurationAndSize(t *testing.T) {
	out := t.TempDir()
	m := newManager(&fakeCutter{}, &fakeResolver{})
	m.cutter = &writingCutter{}
	req := request(2, ModeBoth)
	req.OutputDir = out
	m.Start(req)
	v := waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
	for _, c := range v.Clips {
		if c.Duration != 10 || c.Size != int64(len("clip-bytes")) {
			t.Errorf("clip state %+v; want duration 10 and size of written file", c)
		}
		if _, err := os.Stat(filepath.Join(out, c.File)); err != nil {
			t.Errorf("both-mode clip %s should stay in the output folder: %v", c.File, err)
		}
	}
	if v.Concat.File != ConcatFile {
		t.Errorf("empty ReelFile should default to %s, got %s", ConcatFile, v.Concat.File)
	}
}

// untilCancelled is a cut that only ends when its context does, like a long ffmpeg run.
type untilCancelled struct{ fakeCutter }

func (u *untilCancelled) Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestShutdownStopsRunningWork(t *testing.T) {
	m := NewManager(&fakeResolver{}, &untilCancelled{})
	m.Backoff = []time.Duration{time.Hour} // a pending retry must not delay shutdown
	if _, err := m.Start(request(2, ModeBoth)); err != nil {
		t.Fatal(err)
	}
	if got := m.Running(); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("Running = %v; want [abc]", got)
	}
	if !m.Shutdown(2 * time.Second) {
		t.Fatal("Shutdown timed out with work still running")
	}
	v, _ := m.View("abc")
	if v.Running || v.Clips[0].Status != Failed || v.Clips[1].Status != Failed {
		t.Fatalf("after shutdown: %+v", v)
	}
	if got := m.Running(); len(got) != 0 {
		t.Fatalf("Running after shutdown = %v", got)
	}
	if _, err := m.Start(request(1, ModeClips)); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Start after shutdown: err = %v; want ErrShuttingDown", err)
	}
}

func TestMarkTrashed(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{})
	m.Start(request(2, ModeBoth))
	waitFor(t, m, idle)
	if _, ok := m.MarkTrashed("abc", "nope.mp4"); ok {
		t.Error("unknown file marked")
	}
	if _, ok := m.MarkTrashed("other", "a.mp4"); ok {
		t.Error("unknown session marked")
	}
	v, ok := m.MarkTrashed("abc", "a.mp4")
	if !ok || !v.Clips[0].Trashed || v.Clips[1].Trashed || v.Concat.Trashed {
		t.Fatalf("clip a: ok=%v %+v", ok, v)
	}
	if v, ok = m.MarkTrashed("abc", ConcatFile); !ok || !v.Concat.Trashed {
		t.Fatalf("reel: ok=%v %+v", ok, v.Concat)
	}
}
