package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/krismp/highlighter/internal/combine"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ytdlp"
)

const vid = "jNQXAC9IVRw"

type fakeResolver struct {
	calls   atomic.Int32
	release chan struct{} // when set, Resolve waits for it
	err     error
}

func (f *fakeResolver) Resolve(ctx context.Context, url string) (ytdlp.Info, error) {
	f.calls.Add(1)
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return ytdlp.Info{}, f.err
	}
	return ytdlp.Info{ID: vid, Title: "Me at the zoo", Duration: 19, Width: 320, Height: 240,
		Video: ytdlp.Stream{URL: "https://v"}}, nil
}

// quickTitle stands in for the oEmbed lookup.
type quickTitle struct {
	title string
	err   error
}

func (q *quickTitle) get(ctx context.Context, url string) (string, error) { return q.title, q.err }

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
	quick    *quickTitle
	jobs     *job.Manager
	combiner *combine.Manager
	out      string
	cfg      string
	revealed *[]string // paths passed to RevealPath
	trashed  *[]string // paths passed to TrashPath
	quit     chan struct{}
	internet *error // what the Internet check returns
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, fileCutter{})
}

func newEnvWith(t *testing.T, cutter job.Cutter) *env {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	st, err := store.New(filepath.Join(dir, "config"), store.Settings{OutputRoot: out, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{}
	quick := &quickTitle{err: errors.New("offline")}
	jobs := job.NewManager(res, cutter)
	jobs.Backoff = []time.Duration{0, 0}
	n := 0
	revealed := &[]string{}
	trashed := &[]string{}
	quit := make(chan struct{}, 1)
	internet := new(error)
	combiner := combine.NewManager(combineRunner{})
	h := New(Deps{
		Store: st, Jobs: jobs, Resolver: res, Encoder: ffmpeg.X264, ResolveWait: time.Second, QuickTitle: quick.get,
		Combiner: combiner, Media: fakeMedia{},
		Health:      func(context.Context) tools.Status { return tools.Status{Encoder: "libx264"} },
		UpdateYtDlp: func(context.Context) (string, error) { return "updated", nil },
		OpenPath:    func(string) error { return nil },
		RevealPath:  func(p string) error { *revealed = append(*revealed, p); return nil },
		TrashPath:   func(p string) error { *trashed = append(*trashed, p); return os.Remove(p) },
		Quit:        func() { quit <- struct{}{} },
		Internet:    func(context.Context) error { return *internet },
		NewID:       func() string { n++; return fmt.Sprintf("c%d", n) },
		UI: fstest.MapFS{
			"index.html":    {Data: []byte("<html>app</html>")},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, resolver: res, quick: quick, jobs: jobs, combiner: combiner, out: out, cfg: filepath.Join(dir, "config"), revealed: revealed, trashed: trashed, quit: quit, internet: internet}
}

func (e *env) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	if method != "GET" && method != "HEAD" {
		req.Header.Set("Content-Type", "application/json")
	}
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
	if n := e.resolver.calls.Load(); n != 1 {
		t.Errorf("resolver calls = %d; want 1 (existing session reused)", n)
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
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "clips"}); code != 409 {
		t.Errorf("unapproved process: %d; want 409", code)
	}
}

func TestProcessWritesAndServesFiles(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off\n0:08").Session)
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "both"})
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
	if code, body = e.do(t, "GET", "/api/sessions/"+vid+"/files/me_at_the_zoo.mp4", nil); code != 200 || string(body) != "reel" {
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

func TestRejectsForeignHost(t *testing.T) {
	e := newEnv(t)
	for _, host := range []string{"evil.com:7777", "evil.com", "127.0.0.1.evil.com:80"} {
		req, _ := http.NewRequest("GET", e.srv.URL+"/api/health", nil)
		req.Host = host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 403 || !strings.Contains(string(data), "forbidden host") {
			t.Errorf("host %q: %d %s; want 403 forbidden host", host, resp.StatusCode, data)
		}
	}
}

func TestRequiresJSONContentTypeOnWrites(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/tools/update-ytdlp", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Errorf("text/plain POST: %d; want 415", resp.StatusCode)
	}
	if code, _ := e.do(t, "POST", "/api/tools/update-ytdlp", nil); code != 200 {
		t.Errorf("json POST: %d; want 200", code)
	}
	if code, _ := e.do(t, "GET", "/api/health", nil); code != 200 {
		t.Errorf("GET: %d; want 200", code)
	}
}

func TestPortPinnedHostCheck(t *testing.T) {
	h := hostGuard(7777, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for host, want := range map[string]int{
		"127.0.0.1:7777": 200, "localhost:7777": 200, "127.0.0.1:8888": 403, "evil.com:7777": 403, "127.0.0.1": 403,
	} {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("host %q: %d; want %d", host, rec.Code, want)
		}
	}
}

func TestOpenVideoDoesNotOverwriteCorruptSession(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	path := filepath.Join(e.cfg, "sessions", vid+".json")
	garbage := []byte("{not json")
	if err := os.WriteFile(path, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, data := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://youtu.be/" + vid}); code != 500 {
		t.Errorf("corrupt session: %d %s; want 500", code, data)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, garbage) {
		t.Errorf("session file was overwritten: %s", got)
	}
}

func TestReelOnlyWithCustomName(t *testing.T) {
	e := newEnv(t)
	v := e.open(t)
	if v.ReelName != "me_at_the_zoo" {
		t.Errorf("default reel name = %q; want me_at_the_zoo", v.ReelName)
	}
	v = e.approveAll(t, e.addClips(t, "0:01 tip off\n0:08").Session)
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "everything"}); code != 400 {
		t.Errorf("unknown mode: %d; want 400", code)
	}
	clash := strings.TrimSuffix(v.Clips[0].File, ".mp4")
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "both", "reelName": clash}); code != 400 {
		t.Errorf("reel name equal to a clip file: %d; want 400", code)
	}
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "reel", "reelName": "Final Cut!"})
	if code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	if jv := decode[job.View](t, data); jv.Concat.File != "final_cut.mp4" || jv.Mode != job.ModeReel {
		t.Errorf("job view %+v", jv)
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
	if code, body := e.do(t, "GET", "/api/sessions/"+vid+"/files/final_cut.mp4", nil); code != 200 || string(body) != "reel" {
		t.Errorf("reel: %d %q", code, body)
	}
	if code, _ := e.do(t, "GET", "/api/sessions/"+vid+"/files/"+v.Clips[0].File, nil); code == 200 {
		t.Error("reel-only mode must not leave clip files in the output folder")
	}
}

func TestConnectivity(t *testing.T) {
	e := newEnv(t)
	code, data := e.do(t, "GET", "/api/connectivity", nil)
	if v := decode[map[string]any](t, data); code != 200 || v["online"] != true {
		t.Fatalf("online: %d %s", code, data)
	}
	*e.internet = errors.New("cannot reach YouTube: no route to host")
	code, data = e.do(t, "GET", "/api/connectivity", nil)
	if v := decode[map[string]any](t, data); code != 200 || v["online"] != false || v["error"] != (*e.internet).Error() {
		t.Fatalf("offline: %d %s", code, data)
	}
}

func TestRunningJobsListsTitles(t *testing.T) {
	e := newEnv(t)
	code, data := e.do(t, "GET", "/api/jobs/running", nil)
	if code != 200 || string(bytes.TrimSpace(data)) != `{"running":[]}` {
		t.Fatalf("idle: %d %s", code, data)
	}
}

func TestSettingsRejectTemporaryFolder(t *testing.T) {
	e := newEnv(t)
	code, data := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": filepath.Join(os.TempDir(), "clips"), "concurrency": 2, "defaultDuration": 10})
	if code != 400 || !strings.Contains(string(data), "temporary folder") {
		t.Fatalf("temp folder: %d %s", code, data)
	}
}

func TestPing(t *testing.T) {
	e := newEnv(t)
	code, data := e.do(t, "GET", "/api/ping", nil)
	if code != 200 || decode[map[string]string](t, data)["app"] != "highlighter" {
		t.Fatalf("ping: %d %s", code, data)
	}
}

func TestQuitRespondsThenStops(t *testing.T) {
	e := newEnv(t)
	if code, data := e.do(t, "POST", "/api/quit", nil); code != 204 {
		t.Fatalf("quit: %d %s", code, data)
	}
	select {
	case <-e.quit:
	case <-time.After(2 * time.Second):
		t.Fatal("Quit was not called")
	}
}

func TestTrashFinishedFile(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off\n0:05 dunk").Session)
	if code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "clips"}); code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if jv, _ := e.jobs.View(vid); !jv.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	base := "/api/sessions/" + vid + "/files/"
	code, data := e.do(t, "POST", base+v.Clips[0].File+"/trash", nil)
	if code != 200 {
		t.Fatalf("trash: %d %s", code, data)
	}
	jv := decode[job.View](t, data)
	if !jv.Clips[0].Trashed || jv.Clips[1].Trashed {
		t.Errorf("trashed flags: %+v", jv.Clips)
	}
	want := filepath.Join(e.out, "Me at the zoo", v.Clips[0].File)
	if len(*e.trashed) != 1 || (*e.trashed)[0] != want {
		t.Errorf("trashed %v; want [%s]", *e.trashed, want)
	}
	if code, _ := e.do(t, "POST", base+v.Clips[0].File+"/trash", nil); code != 404 {
		t.Errorf("trash twice: %d; want 404", code)
	}
	if code, _ := e.do(t, "POST", base+"..%2Fsettings.json/trash", nil); code != 400 {
		t.Errorf("path escape: %d; want 400", code)
	}
}

// gatedCutter holds every cut until gate is closed, so a job stays running.
type gatedCutter struct{ gate chan struct{} }

func (g gatedCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, p func(float64)) error {
	<-g.gate
	return os.WriteFile(spec.Output, []byte("clip"), 0o644)
}
func (g gatedCutter) Concat(ctx context.Context, inputs []string, output string) error { return nil }

func TestTrashAndDeleteRefusedWhileProcessing(t *testing.T) {
	gate := make(chan struct{})
	e := newEnvWith(t, gatedCutter{gate})
	defer close(gate)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off").Session)
	if code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "clips"}); code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	if code, data := e.do(t, "POST", "/api/sessions/"+vid+"/files/"+v.Clips[0].File+"/trash", nil); code != 409 {
		t.Errorf("trash while running: %d %s; want 409", code, data)
	}
	if code, data := e.do(t, "DELETE", "/api/sessions/"+vid, nil); code != 409 {
		t.Errorf("delete while running: %d %s; want 409", code, data)
	}
	if len(*e.trashed) != 0 {
		t.Errorf("nothing may be trashed while running: %v", *e.trashed)
	}
}

func TestDeleteSession(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	if code, data := e.do(t, "DELETE", "/api/sessions/"+vid, nil); code != 204 {
		t.Fatalf("delete: %d %s", code, data)
	}
	if code, _ := e.do(t, "GET", "/api/sessions/"+vid, nil); code != 404 {
		t.Errorf("after delete: %d; want 404", code)
	}
	if code, _ := e.do(t, "DELETE", "/api/sessions/"+vid, nil); code != 404 {
		t.Errorf("delete twice: %d; want 404", code)
	}
}

func TestRevealFileInFinder(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off").Session)
	if code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]string{"mode": "clips"}); code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if jv, _ := e.jobs.View(vid); !jv.Running && jv.Clips[0].Status == job.Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	base := "/api/sessions/" + vid + "/files/"
	if code, data := e.do(t, "POST", base+v.Clips[0].File+"/reveal", nil); code != 204 {
		t.Fatalf("reveal: %d %s", code, data)
	}
	want := filepath.Join(e.out, "Me at the zoo", v.Clips[0].File)
	if len(*e.revealed) != 1 || (*e.revealed)[0] != want {
		t.Errorf("revealed %v; want [%s]", *e.revealed, want)
	}
	if code, _ := e.do(t, "POST", base+"missing.mp4/reveal", nil); code != 404 {
		t.Errorf("missing file: %d; want 404", code)
	}
	if code, _ := e.do(t, "POST", base+"notes.txt/reveal", nil); code != 400 {
		t.Errorf("bad name: %d; want 400", code)
	}
	if len(*e.revealed) != 1 {
		t.Errorf("rejected requests must not reveal anything: %v", *e.revealed)
	}
}

func TestDefaultDurationSetting(t *testing.T) {
	e := newEnv(t)
	for _, d := range []float64{0.5, 121} {
		if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "/x", "concurrency": 2, "defaultDuration": d}); code != 400 {
			t.Errorf("defaultDuration %v: %d; want 400", d, code)
		}
	}
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "highlighter-test-clips") // never written: these steps cut nothing
	code, data := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": root, "concurrency": 2})
	if got := decode[store.Settings](t, data); code != 200 || got.DefaultDuration != 10 {
		t.Errorf("missing defaultDuration should save as 10: %d %+v", code, got)
	}
	if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": root, "concurrency": 2, "defaultDuration": 7.5}); code != 200 {
		t.Fatalf("save 7.5: %d", code)
	}
	e.open(t)
	r := e.addClips(t, "0:01: 3PT")
	if c := r.Session.Clips; len(c) != 1 || c[0].Duration != 7.5 {
		t.Errorf("pasted clip should use the default length 7.5: %+v", c)
	}
}
