package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/combine"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), DefaultSettings("/Users/me/Movies"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	s := newStore(t)
	in := Session{VideoID: "jNQXAC9IVRw", URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Title: "Me at the zoo",
		Duration: 19, LeadIn: 1, Clips: []clip.Clip{{ID: "a", Start: 2, Duration: 10}}}
	saved, err := s.SaveSession(in)
	if err != nil || saved.UpdatedAt.IsZero() {
		t.Fatalf("save: %v, %v", saved, err)
	}
	got, err := s.LoadSession("jNQXAC9IVRw")
	if err != nil || got.Title != "Me at the zoo" || len(got.Clips) != 1 || got.LeadIn != 1 {
		t.Fatalf("load: %+v, %v", got, err)
	}
}

func TestLoadMissingAndInvalidIDs(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"aaaaaaaaaaa", "../../etc/x", ""} {
		if _, err := s.LoadSession(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("LoadSession(%q) err = %v; want ErrNotFound", id, err)
		}
	}
	if _, err := s.SaveSession(Session{VideoID: "../evil"}); err == nil {
		t.Error("saving an invalid id must fail")
	}
}

func TestEmptyClipsLoadAsEmptySlice(t *testing.T) {
	s := newStore(t)
	s.SaveSession(Session{VideoID: "jNQXAC9IVRw"})
	got, _ := s.LoadSession("jNQXAC9IVRw")
	if got.Clips == nil {
		t.Error("Clips is nil; want empty slice")
	}
}

func TestRecentSessionsNewestFirst(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3"} {
		at := base.Add(time.Duration(i) * time.Hour)
		s.now = func() time.Time { return at }
		s.SaveSession(Session{VideoID: id})
	}
	got, err := s.RecentSessions(2)
	if err != nil || len(got) != 2 || got[0].VideoID != "aaaaaaaaaa3" || got[1].VideoID != "aaaaaaaaaa2" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSettingsDefaultsAndSave(t *testing.T) {
	s := newStore(t)
	got, err := s.LoadSettings()
	want := Settings{OutputRoot: filepath.Join("/Users/me", "Movies", "Highlights"), Concurrency: 3, DefaultDuration: 10}
	if err != nil || got != want {
		t.Fatalf("defaults = %+v, %v; want %+v", got, err, want)
	}
	clips := filepath.Join("/Users/me", "Desktop", "clips")
	if err := s.SaveSettings(Settings{OutputRoot: clips, Concurrency: 5}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadSettings()
	if got.OutputRoot != clips || got.Concurrency != 5 {
		t.Fatalf("after save = %+v", got)
	}
	// A temporary folder saved by an older version falls back to the default.
	if err := s.SaveSettings(Settings{OutputRoot: filepath.Join(os.TempDir(), "x"), Concurrency: 5}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.LoadSettings(); got.OutputRoot != want.OutputRoot {
		t.Fatalf("temp folder kept: %+v", got)
	}
}

func TestSettingsWithoutDefaultDurationLoadTen(t *testing.T) {
	s := newStore(t)
	// settings.json written before the default length existed
	if err := os.WriteFile(filepath.Join(s.dir, "settings.json"), []byte(`{"outputRoot":"/tmp/x","concurrency":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadSettings()
	if err != nil || got.DefaultDuration != 10 || got.Concurrency != 2 {
		t.Fatalf("got %+v, %v; want default length 10", got, err)
	}
}

func TestDeleteSession(t *testing.T) {
	s := newStore(t)
	if _, err := s.SaveSession(Session{VideoID: "jNQXAC9IVRw", Title: "Me at the zoo"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession("jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadSession("jNQXAC9IVRw"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := s.DeleteSession("jNQXAC9IVRw"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if err := s.DeleteSession("../etc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestCombineDraft(t *testing.T) {
	s, err := New(t.TempDir(), DefaultSettings("/videos"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.LoadCombine()
	if err != nil || len(d.Items) != 0 || d.Items == nil || d.Size != combine.SizeAuto || d.Transition != combine.TransitionCut {
		t.Fatalf("empty draft: %+v %v", d, err)
	}
	d.Items = append(d.Items, CombineItem{ID: "a1", Path: "/v/a.mp4", Media: combine.Media{Duration: 3}})
	d.Name, d.Size, d.Transition = "Finals", combine.SizeVertical, combine.TransitionFade
	if err := s.SaveCombine(d); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadCombine()
	if err != nil || len(got.Items) != 1 || got.Items[0].Media.Duration != 3 || got.Name != "Finals" || got.Size != combine.SizeVertical || got.Transition != combine.TransitionFade {
		t.Fatalf("round trip: %+v %v", got, err)
	}
}
