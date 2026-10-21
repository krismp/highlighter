package server

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/combine"
)

// fakeMedia treats files whose content starts with "video" as 10s 1080p
// videos (with sound unless the name says "silent") and anything else as unreadable.
type fakeMedia struct{}

func (fakeMedia) Probe(ctx context.Context, path string) (combine.Media, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return combine.Media{}, err
	}
	if !strings.HasPrefix(string(data), "video") {
		return combine.Media{}, combine.ErrNoVideo
	}
	silent := strings.Contains(path, "silent")
	return combine.Media{Duration: 10, Width: 1920, Height: 1080, FPS: 30, VideoCodec: "h264", PixFmt: "yuv420p",
		HasAudio: !silent, AudioCodec: "aac", SampleRate: 48000, Channels: "stereo", Size: int64(len(data))}, nil
}

func (fakeMedia) Thumbnail(ctx context.Context, path string, at float64) ([]byte, error) {
	return []byte{0xFF, 0xD8, 0xFF}, nil
}

type combineRunner struct{}

func (combineRunner) Run(ctx context.Context, args []string, onLine func(string)) error {
	return os.WriteFile(args[len(args)-1], []byte("encoded"), 0o644)
}
func (combineRunner) Concat(ctx context.Context, inputs []string, output string) error {
	return os.WriteFile(output, []byte("joined"), 0o644)
}

func (e *env) videos(t *testing.T, names ...string) []string {
	t.Helper()
	dir := filepath.Join(e.out, "Match one")
	os.MkdirAll(dir, 0o755)
	var paths []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		content := "video " + n
		if strings.HasPrefix(n, "notes") {
			content = "hello"
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	return paths
}

type addResponse struct {
	Combine CombineView `json:"combine"`
	Errors  []addError  `json:"errors"`
}

func (e *env) addVideos(t *testing.T, paths ...string) addResponse {
	t.Helper()
	code, data := e.do(t, "POST", "/api/combine/items", map[string]any{"paths": paths})
	if code != 200 {
		t.Fatalf("add: %d %s", code, data)
	}
	return decode[addResponse](t, data)
}

func TestCombineLineUp(t *testing.T) {
	e := newEnv(t)
	code, data := e.do(t, "GET", "/api/combine", nil)
	v := decode[CombineView](t, data)
	if code != 200 || len(v.Items) != 0 || v.Size != combine.SizeAuto || v.Transition != combine.TransitionCut ||
		v.OutputDir != filepath.Join(e.out, "Combined") || !strings.HasPrefix(v.DefaultName, "Highlights ") {
		t.Fatalf("empty: %d %+v", code, v)
	}

	paths := e.videos(t, "01_a.mp4", "02_b.mp4", "silent.mov", "notes.mp4")
	res := e.addVideos(t, append(paths, "relative.mp4")...)
	if len(res.Combine.Items) != 3 || len(res.Errors) != 2 {
		t.Fatalf("add: %+v", res)
	}
	if res.Errors[0].Name != "notes.mp4" || res.Errors[1].Error != "not a full file path" {
		t.Errorf("errors: %+v", res.Errors)
	}
	items := res.Combine.Items
	if items[0].Name != "01_a.mp4" || items[0].Folder != "Match one" || res.Combine.Total != 30 || res.Combine.Copy {
		t.Errorf("items: %+v total %v copy %v", items[0], res.Combine.Total, res.Combine.Copy)
	}

	// Reorder, drop the silent one, change the choices.
	code, data = e.do(t, "PUT", "/api/combine", map[string]any{"itemIds": []string{items[1].ID, items[0].ID},
		"name": " Finals ", "size": "vertical", "transition": "fade"})
	v = decode[CombineView](t, data)
	if code != 200 || len(v.Items) != 2 || v.Items[0].Name != "02_b.mp4" || v.Name != "Finals" ||
		v.Width != 1080 || v.Height != 1920 || v.Transition != combine.TransitionFade {
		t.Fatalf("put: %d %+v", code, v)
	}
	if code, _ := e.do(t, "PUT", "/api/combine", map[string]any{"itemIds": []string{"nope"}, "size": "auto", "transition": "cut"}); code != 409 {
		t.Errorf("unknown id: %d", code)
	}
	if code, _ := e.do(t, "PUT", "/api/combine", map[string]any{"itemIds": []string{}, "size": "8k", "transition": "cut"}); code != 400 {
		t.Errorf("bad size: %d", code)
	}

	// Matching clips without fades are joined by copy.
	e.do(t, "PUT", "/api/combine", map[string]any{"itemIds": []string{items[0].ID, items[1].ID}, "size": "auto", "transition": "cut"})
	_, data = e.do(t, "GET", "/api/combine", nil)
	if v := decode[CombineView](t, data); !v.Copy || v.Width != 1920 {
		t.Errorf("copy plan: %+v", v)
	}

	os.Remove(paths[0])
	_, data = e.do(t, "GET", "/api/combine", nil)
	if v := decode[CombineView](t, data); !v.Items[0].Missing || v.Items[1].Missing {
		t.Errorf("missing file: %+v", v.Items)
	}
	if code, data := e.do(t, "POST", "/api/combine/start", map[string]any{}); code != 409 || !strings.Contains(string(data), "01_a.mp4") {
		t.Errorf("start with a missing file: %d %s", code, data)
	}
}

func waitCombine(t *testing.T, e *env) combine.View {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v := e.combiner.View(); !v.Running && v.Status != combine.Idle {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("combine did not finish")
	return combine.View{}
}

func TestCombineRun(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do(t, "POST", "/api/combine/start", map[string]any{}); code != 400 {
		t.Errorf("start with nothing: %d", code)
	}
	paths := e.videos(t, "a.mp4", "silent.mp4")
	res := e.addVideos(t, paths...)
	e.do(t, "PUT", "/api/combine", map[string]any{"itemIds": []string{res.Combine.Items[0].ID, res.Combine.Items[1].ID},
		"name": "Best: plays.mp4", "size": "720p", "transition": "fade"})

	code, data := e.do(t, "POST", "/api/combine/start", map[string]any{})
	if code != 202 {
		t.Fatalf("start: %d %s", code, data)
	}
	v := waitCombine(t, e)
	want := filepath.Join(e.out, "Combined", "Best- plays.mp4")
	if v.Status != combine.Done || v.Path() != want || v.Copy || v.Width != 1280 {
		t.Fatalf("done: %+v", v)
	}

	code, data = e.do(t, "GET", "/api/combine/result", nil)
	if code != 200 || string(data) != "encoded" {
		t.Errorf("result file: %d %s", code, data)
	}
	if code, _ := e.do(t, "POST", "/api/combine/result/reveal", map[string]any{}); code != 204 || (*e.revealed)[0] != want {
		t.Errorf("reveal: %d %v", code, *e.revealed)
	}
	code, data = e.do(t, "POST", "/api/combine/result/trash", map[string]any{})
	if v := decode[combine.View](t, data); code != 200 || !v.Trashed {
		t.Errorf("trash: %d %s", code, data)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Error("trashed file still there")
	}

	if code, data := e.do(t, "POST", "/api/combine/dismiss", map[string]any{}); code != 200 || len(decode[CombineView](t, data).Items) != 2 || e.combiner.View().Status != combine.Idle {
		t.Errorf("dismiss: %d %s", code, data)
	}

	// A new combination empties the line-up; the source files stay.
	code, data = e.do(t, "DELETE", "/api/combine", nil)
	if v := decode[CombineView](t, data); code != 200 || len(v.Items) != 0 || v.Name != "" || v.Job.Status != combine.Idle || v.Size != combine.Size720 {
		t.Errorf("clear: %d %+v", code, v)
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Error("clearing must not touch the videos")
	}
}

func TestCombineFilesAndLibrary(t *testing.T) {
	e := newEnv(t)
	paths := e.videos(t, "01_a.mp4", "02_b.mp4")
	os.WriteFile(filepath.Join(e.out, "top.mov"), []byte("video top"), 0o644)
	os.MkdirAll(filepath.Join(e.out, ".parts"), 0o755)
	os.WriteFile(filepath.Join(e.out, ".parts", "hidden.mp4"), []byte("video"), 0o644)
	os.WriteFile(filepath.Join(e.out, "Match one", "notes.txt"), []byte("x"), 0o644)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(paths[0], old, old)

	code, data := e.do(t, "GET", "/api/combine/library", nil)
	lib := decode[struct {
		Root  string        `json:"root"`
		Files []LibraryFile `json:"files"`
	}](t, data)
	if code != 200 || lib.Root != e.out || len(lib.Files) != 3 || lib.Files[2].Name != "01_a.mp4" || lib.Files[2].Folder != "Match one" {
		t.Fatalf("library: %d %+v", code, lib)
	}

	code, data = e.do(t, "GET", "/api/combine/library/thumb?path="+url.QueryEscape(paths[1]), nil)
	if code != 200 || data[0] != 0xFF {
		t.Errorf("library thumb: %d", code)
	}
	outside := filepath.Join(filepath.Dir(e.out), "secret.mp4")
	os.WriteFile(outside, []byte("video"), 0o644)
	for _, p := range []string{outside, e.out + "/../secret.mp4", filepath.Join(e.out, "Match one", "notes.txt")} {
		if code, _ := e.do(t, "GET", "/api/combine/library/thumb?path="+url.QueryEscape(p), nil); code != 400 {
			t.Errorf("thumb of %s: %d", p, code)
		}
	}

	res := e.addVideos(t, outside)
	id := res.Combine.Items[0].ID
	if code, data := e.do(t, "GET", "/api/combine/items/"+id+"/file", nil); code != 200 || string(data) != "video" {
		t.Errorf("item file: %d %s", code, data)
	}
	if code, _ := e.do(t, "GET", "/api/combine/items/"+id+"/thumb", nil); code != 200 {
		t.Errorf("item thumb: %d", code)
	}
	if code, _ := e.do(t, "GET", "/api/combine/items/zzz/file", nil); code != 404 {
		t.Errorf("unknown item: %d", code)
	}
	if code, _ := e.do(t, "GET", "/api/combine/result", nil); code != 404 {
		t.Errorf("no result yet: %d", code)
	}
}
