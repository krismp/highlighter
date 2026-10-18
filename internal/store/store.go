// Package store persists sessions and settings as JSON files.
package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/desktop"
)

var ErrNotFound = errors.New("session not found")

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Session is everything known about one video being clipped.
type Session struct {
	VideoID   string      `json:"videoId"`
	URL       string      `json:"url"`
	Title     string      `json:"title"`
	Duration  float64     `json:"duration"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	LeadIn    float64     `json:"leadIn"`
	Clips     []clip.Clip `json:"clips"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type Settings struct {
	OutputRoot      string  `json:"outputRoot"`
	Concurrency     int     `json:"concurrency"`
	DefaultDuration float64 `json:"defaultDuration"` // length of newly added clips, seconds
}

// DefaultSettings saves clips under videosDir (~/Movies on macOS, ~/Videos on Windows).
func DefaultSettings(videosDir string) Settings {
	return Settings{OutputRoot: filepath.Join(videosDir, "Highlights"), Concurrency: 3, DefaultDuration: clip.DefaultDuration}
}

type Store struct {
	dir      string
	defaults Settings
	now      func() time.Time
	mu       sync.Mutex
}

func New(dir string, defaults Settings) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return nil, err
	}
	defaults.DefaultDuration = cmp.Or(defaults.DefaultDuration, clip.DefaultDuration)
	return &Store{dir: dir, defaults: defaults, now: time.Now}, nil
}

func (s *Store) sessionPath(id string) (string, bool) {
	if !validID.MatchString(id) {
		return "", false
	}
	return filepath.Join(s.dir, "sessions", id+".json"), true
}

func (s *Store) SaveSession(sess Session) (Session, error) {
	path, ok := s.sessionPath(sess.VideoID)
	if !ok {
		return Session{}, fmt.Errorf("invalid video id %q", sess.VideoID)
	}
	sess.UpdatedAt = s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	return sess, writeJSONFile(path, sess)
}

func (s *Store) LoadSession(id string) (Session, error) {
	path, ok := s.sessionPath(id)
	if !ok {
		return Session{}, ErrNotFound
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return Session{}, fmt.Errorf("read session %s: %w", id, err)
	}
	if sess.Clips == nil {
		sess.Clips = []clip.Clip{}
	}
	return sess, nil
}

// DeleteSession forgets a session (its clips' start times and labels). The
// clip files it produced are not touched.
func (s *Store) DeleteSession(id string) error {
	path, ok := s.sessionPath(id)
	if !ok {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

func (s *Store) RecentSessions(limit int) ([]Session, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return nil, err
	}
	out := []Session{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		if sess, err := s.LoadSession(id); err == nil {
			out = append(out, sess)
		}
	}
	slices.SortFunc(out, func(a, b Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) LoadSettings() (Settings, error) {
	st := s.defaults
	data, err := os.ReadFile(filepath.Join(s.dir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return s.defaults, fmt.Errorf("read settings: %w", err)
	}
	st.OutputRoot = cmp.Or(st.OutputRoot, s.defaults.OutputRoot)
	if desktop.IsTemporary(st.OutputRoot) {
		st.OutputRoot = s.defaults.OutputRoot // never somewhere the system empties
	}
	st.Concurrency = cmp.Or(st.Concurrency, s.defaults.Concurrency)
	st.DefaultDuration = cmp.Or(st.DefaultDuration, s.defaults.DefaultDuration)
	return st, nil
}

func (s *Store) SaveSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(filepath.Join(s.dir, "settings.json"), st)
}

// writeJSONFile writes atomically via a temp file + rename.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
