package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/job"
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

// Real users move finished clips out of the output folder, so approval stands
// in for the files on old sessions.
func TestVideoStatusOldSessionApproval(t *testing.T) {
	dir := t.TempDir() // no files
	approved := store.Session{Title: "Game 1", Clips: []clip.Clip{{ID: "a", Start: 5, Duration: 10, Approved: true}, {ID: "b", Start: 30, Duration: 10, Approved: true}}}
	if got := videoStatus(approved, false, dir); got != StatusDone {
		t.Errorf("all approved, no files: %q; want done", got)
	}
	partly := approved
	partly.Clips = []clip.Clip{approved.Clips[0], {ID: "b", Start: 30, Duration: 10}}
	if got := videoStatus(partly, false, dir); got != StatusDraft {
		t.Errorf("one unapproved, no files: %q; want draft", got)
	}
}

func TestWithJobOutcome(t *testing.T) {
	t0 := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	ok := job.View{StartedAt: t0.Add(time.Minute), Clips: []job.ClipState{{Status: job.Done}}}
	sess := store.Session{Clips: []clip.Clip{{ID: "a"}}, ChangedAt: t0}
	if got := withJobOutcome(sess, ok).ProcessedAt; !got.Equal(ok.StartedAt) {
		t.Errorf("succeeded job: ProcessedAt %v", got)
	}
	if got := withJobOutcome(sess, job.View{StartedAt: t0.Add(time.Minute), Clips: []job.ClipState{{Status: job.Failed}}}).ProcessedAt; !got.IsZero() {
		t.Errorf("failed job: ProcessedAt %v", got)
	}
	running := ok
	running.Running = true
	if got := withJobOutcome(sess, running).ProcessedAt; !got.IsZero() {
		t.Errorf("running job: ProcessedAt %v", got)
	}
	edited := sess
	edited.ChangedAt = t0.Add(2 * time.Minute)
	if got := withJobOutcome(edited, ok).ProcessedAt; !got.IsZero() {
		t.Errorf("edit after start: ProcessedAt %v", got)
	}
}
