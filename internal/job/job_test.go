package job

import (
	"context"
	"errors"
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

func request(n int, concat bool) Request {
	req := Request{SessionID: "abc", URL: "https://www.youtube.com/watch?v=abc", OutputDir: "/out",
		Concat: concat, Encoder: ffmpeg.X264, Concurrency: 3}
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
	if _, err := m.Start(request(2, false)); err != nil {
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
	if got.Start != 10 || got.Duration != 10 || got.Output != "/out/b.mp4" || got.Encoder != ffmpeg.X264 ||
		got.Video.URL != "https://v" || got.Audio.URL != "https://a" {
		t.Errorf("cut spec %+v", got)
	}
}

func TestRetriesTransientFailuresAndReResolves(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"a.mp4": 2}}
	r := &fakeResolver{}
	m := newManager(c, r)
	m.Start(request(1, false))
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
	m.Start(request(2, false))
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
	m.Start(request(1, false))
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
	m.Start(request(1, false))
	v := waitFor(t, m, idle)
	if s := v.Clips[0]; s.Status != Failed || s.Error != "yt-dlp: ERROR: Private video" {
		t.Errorf("state %+v", s)
	}
}

func TestRespectsConcurrency(t *testing.T) {
	c := &fakeCutter{delay: 20 * time.Millisecond}
	m := newManager(c, &fakeResolver{})
	req := request(6, false)
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
	m.Start(request(3, true))
	v := waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
	want := []string{"/out/highlights_full.mp4", "/out/a.mp4", "/out/b.mp4", "/out/c.mp4"}
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
	m.Start(request(2, true))
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
	m.Start(request(1, false))
	if _, err := m.Start(request(1, false)); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v; want ErrBusy", err)
	}
	close(c.gate)
	waitFor(t, m, idle)
	if _, err := m.Start(request(1, false)); err != nil {
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
	m.Start(request(2, false))
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
		m.Start(request(1, false))
		waitFor(t, m, idle)
		c.fail.Store(false)

		var wg sync.WaitGroup
		var retryErr, startErr error
		wg.Add(2)
		go func() { defer wg.Done(); _, retryErr = m.Retry("abc", "a") }()
		go func() { defer wg.Done(); _, startErr = m.Start(request(1, false)) }()
		wg.Wait()
		if retryErr == nil && startErr == nil {
			t.Fatal("Retry and Start both succeeded: two jobs running for one session")
		}
		close(c.gate)
		waitFor(t, m, idle)
	}
}
