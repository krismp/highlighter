package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ytdlp"
)

const vid = "jNQXAC9IVRw"

type fakeResolver struct{ calls int }

func (f *fakeResolver) Resolve(ctx context.Context, url string) (ytdlp.Info, error) {
	f.calls++
	return ytdlp.Info{ID: vid, Title: "Me at the zoo", Duration: 19, Width: 320, Height: 240,
		Video: ytdlp.Stream{URL: "https://v"}}, nil
}

type fileCutter struct{}

func (fileCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, p func(float64)) error {
	return os.WriteFile(spec.Output, []byte("clip"), 0o644)
}
func (fileCutter) Concat(ctx context.Context, inputs []string, output string) error {
	return os.WriteFile(output, []byte("reel"), 0o644)
}

type env struct {
	srv      *httptest.Server
	resolver *fakeResolver
	jobs     *job.Manager
	out      string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	st, err := store.New(filepath.Join(dir, "config"), store.Settings{OutputRoot: out, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{}
	jobs := job.NewManager(res, fileCutter{})
	jobs.Backoff = []time.Duration{0, 0}
	n := 0
	h := New(Deps{
		Store: st, Jobs: jobs, Resolver: res, Encoder: ffmpeg.X264,
		Health:      func(context.Context) tools.Status { return tools.Status{Encoder: "libx264"} },
		UpdateYtDlp: func(context.Context) (string, error) { return "updated", nil },
		OpenPath:    func(string) error { return nil },
		NewID:       func() string { n++; return fmt.Sprintf("c%d", n) },
		UI: fstest.MapFS{
			"index.html":    {Data: []byte("<html>app</html>")},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, resolver: res, jobs: jobs, out: out}
}

func (e *env) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

func (e *env) open(t *testing.T) SessionView {
	code, data := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://youtu.be/" + vid + "?si=x"})
	if code != 200 {
		t.Fatalf("open: %d %s", code, data)
	}
	return decode[SessionView](t, data)
}

func (e *env) addClips(t *testing.T, text string) parseResponse {
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/clips/parse", map[string]string{"text": text})
	if code != 200 {
		t.Fatalf("parse: %d %s", code, data)
	}
	return decode[parseResponse](t, data)
}

func (e *env) approveAll(t *testing.T, v SessionView) SessionView {
	clips := make([]map[string]any, len(v.Clips))
	for i, c := range v.Clips {
		clips[i] = map[string]any{"id": c.ID, "start": c.Start, "duration": c.Duration, "label": c.Label, "approved": true}
	}
	code, data := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": v.LeadIn, "clips": clips})
	if code != 200 {
		t.Fatalf("put: %d %s", code, data)
	}
	return decode[SessionView](t, data)
}

func TestOpenVideo(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://vimeo.com/1"}); code != 400 {
		t.Errorf("non-youtube: %d; want 400", code)
	}
	v := e.open(t)
	if v.VideoID != vid || v.Title != "Me at the zoo" || v.DurationText != "0:19" || v.Clips == nil || v.Issues == nil || v.CanProcess {
		t.Errorf("view %+v", v)
	}
	if !strings.HasSuffix(v.OutputDir, filepath.Join("out", "Me at the zoo")) {
		t.Errorf("outputDir %q", v.OutputDir)
	}
	e.open(t)
	if e.resolver.calls != 1 {
		t.Errorf("resolver calls = %d; want 1 (existing session reused)", e.resolver.calls)
	}
	code, data := e.do(t, "GET", "/api/sessions", nil)
	recent := decode[[]RecentSession](t, data)
	if code != 200 || len(recent) != 1 || recent[0].VideoID != vid {
		t.Errorf("recent %d %s", code, data)
	}
}

func TestParseClips(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	r := e.addClips(t, "0:05 Andi crossover\nbanana\n0:02")
	if len(r.Errors) != 1 || r.Errors[0].Line != 2 {
		t.Errorf("errors %+v", r.Errors)
	}
	c := r.Session.Clips
	if len(c) != 2 || c[0].Start != 2 || c[1].Start != 5 || c[1].Label != "Andi crossover" || c[0].Duration != 10 {
		t.Fatalf("clips %+v", c)
	}
	if c[0].Index != 1 || c[1].File != "02_0-05_andi-crossover.mp4" || c[1].StartText != "0:05" || c[1].EffDuration != 10 {
		t.Errorf("clip view %+v", c[1])
	}
	// 2–12 overlaps 5–15 → exactly one overlap warning on the second clip; nothing past 0:19
	if len(r.Session.Issues) != 1 || r.Session.Issues[0].ClipID != c[1].ID || r.Session.Issues[0].Severity != "warning" {
		t.Errorf("issues %+v", r.Session.Issues)
	}
}

func TestApprovalResetsOnTimingChange(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01\n0:08").Session)
	if !v.CanProcess {
		t.Fatalf("all approved should be processable: %+v", v)
	}
	clips := []map[string]any{
		{"id": v.Clips[0].ID, "start": 1, "duration": 6, "label": "", "approved": true},
		{"id": v.Clips[1].ID, "start": 8, "duration": 10, "label": "renamed", "approved": true},
	}
	_, data := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": 0, "clips": clips})
	v = decode[SessionView](t, data)
	if v.Clips[0].Approved || !v.Clips[1].Approved || v.CanProcess {
		t.Errorf("after edit %+v", v.Clips)
	}
	if code, _ := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": 31, "clips": clips}); code != 400 {
		t.Errorf("lead-in 31: %d; want 400", code)
	}
}

func TestProcessRequiresApproval(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	e.addClips(t, "0:01")
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]bool{"concat": false}); code != 409 {
		t.Errorf("unapproved process: %d; want 409", code)
	}
}

func TestProcessWritesAndServesFiles(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off\n0:08").Session)
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]bool{"concat": true})
	if code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if jv, _ := e.jobs.View(vid); jv.Concat.Status == job.Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	code, body := e.do(t, "GET", "/api/sessions/"+vid+"/files/"+v.Clips[0].File, nil)
	if code != 200 || string(body) != "clip" {
		t.Errorf("file: %d %q", code, body)
	}
	if code, body = e.do(t, "GET", "/api/sessions/"+vid+"/files/highlights_full.mp4", nil); code != 200 || string(body) != "reel" {
		t.Errorf("reel: %d %q", code, body)
	}
	dir := filepath.Join(e.out, "Me at the zoo")
	for path, content := range map[string]string{
		filepath.Join(e.out, "secret.mp4"): "SECRET",
		filepath.Join(dir, "notes.txt"):    "SECRET",
		filepath.Join(dir, ".hidden.mp4"):  "SECRET",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"..%2Fsecret.mp4", "notes.txt", ".hidden.mp4"} {
		code, body := e.do(t, "GET", "/api/sessions/"+vid+"/files/"+bad, nil)
		if code != 400 || strings.Contains(string(body), "SECRET") {
			t.Errorf("file %q: %d %q; want 400 without contents", bad, code, body)
		}
	}
	// SSE sends the current snapshot first
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/sessions/"+vid+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			jv := decode[job.View](t, []byte(line))
			if jv.Running || jv.Clips[0].Status != job.Done {
				t.Errorf("snapshot %+v", jv)
			}
			return
		}
	}
	t.Fatal("no SSE data line")
}

func TestSettings(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "/x", "concurrency": 0}); code != 400 {
		t.Errorf("concurrency 0: %d", code)
	}
	if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "relative/x", "concurrency": 2}); code != 400 {
		t.Errorf("relative root: %d", code)
	}
	code, data := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "~/Clips", "concurrency": 4})
	home, _ := os.UserHomeDir()
	got := decode[store.Settings](t, data)
	if code != 200 || got.OutputRoot != filepath.Join(home, "Clips") || got.Concurrency != 4 {
		t.Errorf("settings %d %+v", code, got)
	}
}

func TestSPAAndUnknownAPI(t *testing.T) {
	e := newEnv(t)
	if code, body := e.do(t, "GET", "/s/"+vid, nil); code != 200 || string(body) != "<html>app</html>" {
		t.Errorf("spa fallback: %d %q", code, body)
	}
	if code, body := e.do(t, "GET", "/assets/app.js", nil); code != 200 || string(body) != "console.log(1)" {
		t.Errorf("asset: %d %q", code, body)
	}
	if code, _ := e.do(t, "GET", "/api/nope", nil); code != 404 {
		t.Errorf("unknown api: %d; want 404", code)
	}
	if code, _ := e.do(t, "GET", "/api/sessions/aaaaaaaaaaa", nil); code != 404 {
		t.Errorf("missing session: %d; want 404", code)
	}
}
