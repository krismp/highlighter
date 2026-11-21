package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/store"
)

func with(s store.Session, changedAt, processedAt time.Time) store.Session {
	s.ChangedAt, s.ProcessedAt = changedAt, processedAt
	return s
}

func withDraft(s store.Session, text string) store.Session {
	s.Draft = text
	return s
}

func TestVideoStatus(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	withClips := store.Session{Title: "Game 1", Clips: []clip.Clip{{ID: "a", Start: 5, Duration: 10}}}
	cases := []struct {
		name    string
		sess    store.Session
		running bool
		want    Status
	}{
		{"running wins", withClips, true, StatusProcessing},
		{"no clips, no text", store.Session{}, false, StatusEmpty},
		{"only paste text", store.Session{Draft: "1:23"}, false, StatusDraft},
		{"changed after run", with(withClips, t0.Add(time.Minute), t0), false, StatusDraft},
		{"run after change", with(withClips, t0, t0.Add(time.Minute)), false, StatusDone},
		{"done but text left", withDraft(with(withClips, t0, t0.Add(time.Minute)), "2:00"), false, StatusDraft},
		{"old, no files", withClips, false, StatusDraft},
	}
	for _, c := range cases {
		if got := videoStatus(c.sess, c.running, dir); got != c.want {
			t.Errorf("%s: %q; want %q", c.name, got, c.want)
		}
	}
}

// Old sessions have no timestamps: their files on disk decide.
func TestVideoStatusOldSessionFiles(t *testing.T) {
	sess := store.Session{Title: "Game 1", Clips: []clip.Clip{{ID: "a", Start: 5, Duration: 10}, {ID: "b", Start: 30, Duration: 10}}}
	clipsDir, reelDir := t.TempDir(), t.TempDir()
	for i, c := range sess.Clips {
		os.WriteFile(filepath.Join(clipsDir, clip.Filename(i+1, c)), nil, 0o644)
	}
	os.WriteFile(filepath.Join(reelDir, clip.ReelName(sess.Title)+".mp4"), nil, 0o644) // "reel only": no clip files
	if got := videoStatus(sess, false, clipsDir); got != StatusDone {
		t.Errorf("every clip file: %q; want done", got)
	}
	if got := videoStatus(sess, false, reelDir); got != StatusDone {
		t.Errorf("reel file: %q; want done", got)
	}
}
