package server

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/ytdlp"
)

func (e *env) waitResolved(t *testing.T) SessionView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, data := e.do(t, "GET", "/api/sessions/"+vid, nil)
		if v := decode[SessionView](t, data); !v.Resolving {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("details never arrived")
	return SessionView{}
}

// The page opens before yt-dlp is done; clips added meanwhile survive the
// details arriving, and processing waits for them.
func TestOpenVideoBeforeDetails(t *testing.T) {
	e := newEnv(t)
	e.resolver.release = make(chan struct{})
	e.quick.title, e.quick.err = "Me at the zoo (oEmbed)", nil

	start := time.Now() // the open waits at most ResolveWait (1s in tests)
	v := e.open(t)
	if !v.Resolving || v.Title != "Me at the zoo (oEmbed)" || v.Duration != 0 || v.CanProcess {
		t.Fatalf("opened: %+v", v)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("opening waited for yt-dlp")
	}
	e.addClips(t, "0:05 first\n0:30 past the end")
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]any{"mode": "clips"}); code != 409 {
		t.Errorf("process while resolving: %d", code)
	}
	if v := e.open(t); !v.Resolving || e.resolver.calls.Load() != 1 {
		t.Errorf("reopening must reuse the running lookup: %+v calls %d", v, e.resolver.calls.Load())
	}

	close(e.resolver.release)
	v = e.waitResolved(t)
	if v.DurationText != "0:19" || len(v.Clips) != 2 || v.Title != "Me at the zoo (oEmbed)" || v.ResolveError != "" {
		t.Fatalf("resolved: %+v", v)
	}
	if !strings.Contains(v.Issues[0].Message, "after the video ends") {
		t.Errorf("the duration must now validate clips: %+v", v.Issues)
	}
}

func TestOpenVideoDetailsFail(t *testing.T) {
	e := newEnv(t)
	e.resolver.err = errors.New("yt-dlp: ERROR: Sign in to confirm your age")
	v := e.open(t)
	if v.Resolving || !strings.Contains(v.ResolveError, "age") || v.Title != "YouTube video "+vid || v.CanProcess {
		t.Fatalf("failed: %+v", v)
	}
	if code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]any{"mode": "clips"}); code != 409 || !strings.Contains(string(data), "age") {
		t.Errorf("process after failure: %d %s", code, data)
	}
	e.resolver.err = nil
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/resolve", map[string]any{})
	if code != 200 {
		t.Fatalf("retry: %d %s", code, data)
	}
	if v := e.waitResolved(t); v.ResolveError != "" || v.DurationText != "0:19" || v.Title != "Me at the zoo" {
		t.Errorf("after retry: %+v", v)
	}
}

func TestOpenVideoThatDoesNotExist(t *testing.T) {
	e := newEnv(t)
	e.quick.err = ytdlp.ErrNoVideo
	code, data := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://youtu.be/" + vid})
	if code != 404 || !strings.Contains(string(data), "no YouTube video") || e.resolver.calls.Load() != 0 {
		t.Errorf("missing video: %d %s", code, data)
	}
	if code, _ := e.do(t, "GET", "/api/sessions/"+vid, nil); code != 404 {
		t.Errorf("no session must be saved: %d", code)
	}
}
