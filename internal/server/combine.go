package server

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/combine"
	"github.com/krismp/highlighter/internal/store"
)

// MediaTool looks into local video files (combine.Tool in production).
type MediaTool interface {
	Probe(ctx context.Context, path string) (combine.Media, error)
	Thumbnail(ctx context.Context, path string, at float64) ([]byte, error)
}

const (
	maxCombineItems = 200
	combinedDir     = "Combined" // in the output folder
	libraryLimit    = 60
)

// videoExts are the files offered from the output folder and accepted by drops.
var videoExts = []string{".mp4", ".mov", ".m4v", ".mkv", ".webm", ".avi", ".mts", ".m2ts", ".3gp", ".wmv", ".mpg", ".mpeg", ".ts"}

func isVideoName(name string) bool {
	return slices.Contains(videoExts, strings.ToLower(filepath.Ext(name)))
}

type CombineItemView struct {
	ID      string        `json:"id"`
	Path    string        `json:"path"`
	Name    string        `json:"name"`   // file name
	Folder  string        `json:"folder"` // the folder it is in, for telling same-named clips apart
	Media   combine.Media `json:"media"`
	Missing bool          `json:"missing"` // moved, renamed or deleted since it was added
}

type CombineView struct {
	Items       []CombineItemView  `json:"items"`
	Name        string             `json:"name"`
	DefaultName string             `json:"defaultName"`
	Size        combine.Size       `json:"size"`
	Transition  combine.Transition `json:"transition"`
	OutputDir   string             `json:"outputDir"`
	Total       float64            `json:"total"` // seconds
	Width       int                `json:"width"` // frame of the result with the current choices
	Height      int                `json:"height"`
	Copy        bool               `json:"copy"` // would join without re-encoding
	Job         combine.View       `json:"job"`
}

type LibraryFile struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Folder  string    `json:"folder"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

type addError struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

func defaultCombineName(now time.Time) string { return "Highlights " + now.Format("2006-01-02") }

func inputsOf(items []store.CombineItem) []combine.Input {
	in := make([]combine.Input, len(items))
	for i, it := range items {
		in[i] = combine.Input{Path: it.Path, Media: it.Media}
	}
	return in
}

func (s *server) combineView(d store.CombineDraft, outputRoot string) CombineView {
	v := CombineView{
		Items: make([]CombineItemView, 0, len(d.Items)), Name: d.Name, DefaultName: defaultCombineName(time.Now()),
		Size: d.Size, Transition: d.Transition, OutputDir: filepath.Join(outputRoot, combinedDir), Job: s.Combiner.View(),
	}
	for _, it := range d.Items {
		_, err := os.Stat(it.Path)
		v.Items = append(v.Items, CombineItemView{ID: it.ID, Path: it.Path, Name: filepath.Base(it.Path),
			Folder: filepath.Base(filepath.Dir(it.Path)), Media: it.Media, Missing: err != nil})
		v.Total += it.Media.Duration
	}
	p := combine.NewPlan(inputsOf(d.Items), d.Size, d.Transition)
	v.Width, v.Height, v.Copy = p.Width, p.Height, p.Copy && len(d.Items) > 1
	return v
}

func (s *server) loadCombine(w http.ResponseWriter) (store.CombineDraft, bool) {
	d, err := s.Store.LoadCombine()
	if err != nil {
		writeError(w, 500, err.Error())
		return d, false
	}
	return d, true
}

func (s *server) respondCombine(w http.ResponseWriter, d store.CombineDraft, wrap func(CombineView) any) {
	st, ok := s.settings(w)
	if !ok {
		return
	}
	v := s.combineView(d, st.OutputRoot)
	if wrap != nil {
		writeJSON(w, 200, wrap(v))
		return
	}
	writeJSON(w, 200, v)
}

func (s *server) saveCombine(w http.ResponseWriter, d store.CombineDraft, wrap func(CombineView) any) {
	if err := s.Store.SaveCombine(d); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	s.respondCombine(w, d, wrap)
}

func (s *server) getCombine(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.loadCombine(w); ok {
		s.respondCombine(w, d, nil)
	}
}

// putCombine reorders and removes videos (itemIds lists the ones to keep, in
// their new order) and saves the name, size and transition.
func (s *server) putCombine(w http.ResponseWriter, r *http.Request) {
	d, ok := s.loadCombine(w)
	if !ok {
		return
	}
	var body struct {
		ItemIDs    []string           `json:"itemIds"`
		Name       string             `json:"name"`
		Size       combine.Size       `json:"size"`
		Transition combine.Transition `json:"transition"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !body.Size.Valid() || !body.Transition.Valid() {
		writeError(w, 400, "unknown size or transition")
		return
	}
	byID := map[string]store.CombineItem{}
	for _, it := range d.Items {
		byID[it.ID] = it
	}
	items := make([]store.CombineItem, 0, len(body.ItemIDs))
	for _, id := range body.ItemIDs {
		it, ok := byID[id]
		if !ok {
			writeError(w, 409, "the list of videos changed; reload the page")
			return
		}
		delete(byID, id) // each item once
		items = append(items, it)
	}
	d.Items, d.Name, d.Size, d.Transition = items, strings.TrimSpace(body.Name), body.Size, body.Transition
	s.saveCombine(w, d, nil)
}

// addCombineItems probes each path and appends the videos to the line-up;
// files that are not readable videos come back as errors, the rest are added.
func (s *server) addCombineItems(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	d, ok := s.loadCombine(w)
	if !ok {
		return
	}
	errs := []addError{}
	for _, p := range body.Paths {
		name := filepath.Base(p)
		if !filepath.IsAbs(p) {
			errs = append(errs, addError{p, name, "not a full file path"})
			continue
		}
		if len(d.Items) >= maxCombineItems {
			errs = append(errs, addError{p, name, fmt.Sprintf("at most %d videos can be combined at once", maxCombineItems)})
			continue
		}
		m, err := s.Media.Probe(r.Context(), p)
		if err != nil {
			errs = append(errs, addError{p, name, err.Error()})
			continue
		}
		d.Items = append(d.Items, store.CombineItem{ID: s.NewID(), Path: filepath.Clean(p), Media: m})
	}
	s.saveCombine(w, d, func(v CombineView) any { return map[string]any{"combine": v, "errors": errs} })
}

// clearCombine starts a new combination: it empties the line-up and forgets
// the last result. No file is touched.
func (s *server) clearCombine(w http.ResponseWriter, r *http.Request) {
	if !s.Combiner.Reset() {
		writeError(w, 409, "Videos are still being combined. Cancel first, or wait until it finishes.")
		return
	}
	d, ok := s.loadCombine(w)
	if !ok {
		return
	}
	d.Items, d.Name = []store.CombineItem{}, ""
	s.saveCombine(w, d, nil)
}

func (s *server) startCombine(w http.ResponseWriter, r *http.Request) {
	d, ok := s.loadCombine(w)
	if !ok {
		return
	}
	st, ok := s.settings(w)
	if !ok {
		return
	}
	if len(d.Items) < 2 {
		writeError(w, 400, "Add at least two videos to combine.")
		return
	}
	// Probe again: a file may have been replaced or deleted since it was added.
	for i, it := range d.Items {
		m, err := s.Media.Probe(r.Context(), it.Path)
		if err != nil {
			writeError(w, 409, fmt.Sprintf("Video %d, %s, can't be read anymore (%v). Remove it or add it again.", i+1, filepath.Base(it.Path), err))
			return
		}
		d.Items[i].Media = m
	}
	if err := s.Store.SaveCombine(d); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	name := clip.SafeDirName(strings.TrimSuffix(cmp.Or(d.Name, defaultCombineName(time.Now())), ".mp4"))
	v, err := s.Combiner.Start(combine.Request{Inputs: inputsOf(d.Items), Size: d.Size, Transition: d.Transition,
		Encoder: s.Encoder, Dir: filepath.Join(st.OutputRoot, combinedDir), Name: name})
	switch {
	case errors.Is(err, combine.ErrBusy):
		writeError(w, 409, err.Error())
	case err != nil:
		writeError(w, 500, err.Error())
	default:
		writeJSON(w, 202, v)
	}
}

// dismissCombine forgets the last result (the file stays) so the line-up can
// be changed and combined again.
func (s *server) dismissCombine(w http.ResponseWriter, r *http.Request) {
	if !s.Combiner.Reset() {
		writeError(w, 409, "Videos are still being combined. Cancel first, or wait until it finishes.")
		return
	}
	if d, ok := s.loadCombine(w); ok {
		s.respondCombine(w, d, nil)
	}
}

func (s *server) cancelCombine(w http.ResponseWriter, r *http.Request) {
	s.Combiner.Cancel()
	w.WriteHeader(204)
}

func (s *server) combineEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}
	current, updates, cancel := s.Combiner.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	send := func(v combine.View) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	send(current)
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-updates:
			send(v)
		}
	}
}

func (s *server) combineItem(w http.ResponseWriter, r *http.Request) (store.CombineItem, bool) {
	d, ok := s.loadCombine(w)
	if !ok {
		return store.CombineItem{}, false
	}
	i := slices.IndexFunc(d.Items, func(it store.CombineItem) bool { return it.ID == r.PathValue("itemId") })
	if i < 0 {
		writeError(w, 404, "no such video in the list")
		return store.CombineItem{}, false
	}
	return d.Items[i], true
}

// combineItemFile plays a lined-up video in the page's preview.
func (s *server) combineItemFile(w http.ResponseWriter, r *http.Request) {
	if it, ok := s.combineItem(w, r); ok {
		http.ServeFile(w, r, it.Path)
	}
}

func (s *server) combineItemThumb(w http.ResponseWriter, r *http.Request) {
	if it, ok := s.combineItem(w, r); ok {
		s.serveThumb(w, r, it.Path)
	}
}

// libraryThumb is a thumbnail of a file in the output folder.
func (s *server) libraryThumb(w http.ResponseWriter, r *http.Request) {
	st, ok := s.settings(w)
	if !ok {
		return
	}
	p := filepath.Clean(r.URL.Query().Get("path"))
	rel, err := filepath.Rel(st.OutputRoot, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(p) || !isVideoName(p) {
		writeError(w, 400, "not a video in the output folder")
		return
	}
	s.serveThumb(w, r, p)
}

func (s *server) serveThumb(w http.ResponseWriter, r *http.Request, path string) {
	fi, err := os.Stat(path)
	if err != nil {
		writeError(w, 404, "file not found: "+filepath.Base(path))
		return
	}
	key := fmt.Sprintf("%s|%d|%d", path, fi.Size(), fi.ModTime().UnixNano())
	jpg, ok := s.thumbs.get(key)
	if !ok {
		at := 1.0
		if m, err := s.Media.Probe(r.Context(), path); err == nil {
			at = min(1, m.Duration/3) // a frame past any fade-in, inside short clips
		}
		if jpg, err = s.Media.Thumbnail(r.Context(), path, at); err != nil {
			writeError(w, 500, err.Error())
			return
		}
		s.thumbs.put(key, jpg)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "thumb.jpg", fi.ModTime(), bytes.NewReader(jpg))
}

// library lists the newest videos in the output folder and the folders
// directly inside it (one per match, and Combined), for adding with one click.
func (s *server) library(w http.ResponseWriter, r *http.Request) {
	st, ok := s.settings(w)
	if !ok {
		return
	}
	files := []LibraryFile{}
	add := func(dir string) []os.DirEntry {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !isVideoName(e.Name()) {
				continue
			}
			if fi, err := e.Info(); err == nil && fi.Size() > 0 {
				files = append(files, LibraryFile{Path: filepath.Join(dir, e.Name()), Name: e.Name(),
					Folder: filepath.Base(dir), Size: fi.Size(), ModTime: fi.ModTime().UTC()})
			}
		}
		return entries
	}
	for _, e := range add(st.OutputRoot) {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			add(filepath.Join(st.OutputRoot, e.Name()))
		}
	}
	slices.SortFunc(files, func(a, b LibraryFile) int { return b.ModTime.Compare(a.ModTime) })
	if len(files) > libraryLimit {
		files = files[:libraryLimit]
	}
	writeJSON(w, 200, map[string]any{"root": st.OutputRoot, "files": files})
}

// combineResult resolves the finished combined file.
func (s *server) combineResult(w http.ResponseWriter) (string, bool) {
	v := s.Combiner.View()
	if v.Status != combine.Done || v.Trashed {
		writeError(w, 404, "there is no combined video yet")
		return "", false
	}
	if _, err := os.Stat(v.Path()); err != nil {
		writeError(w, 404, "file not found: "+v.File)
		return "", false
	}
	return v.Path(), true
}

func (s *server) combineResultFile(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.combineResult(w); ok {
		http.ServeFile(w, r, p)
	}
}

func (s *server) revealCombineResult(w http.ResponseWriter, r *http.Request) {
	p, ok := s.combineResult(w)
	if !ok {
		return
	}
	if err := s.RevealPath(p); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *server) trashCombineResult(w http.ResponseWriter, r *http.Request) {
	p, ok := s.combineResult(w)
	if !ok {
		return
	}
	if err := s.TrashPath(p); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.Combiner.MarkTrashed())
}

func (s *server) openCombineFolder(w http.ResponseWriter, r *http.Request) {
	st, ok := s.settings(w)
	if !ok {
		return
	}
	dir := filepath.Join(st.OutputRoot, combinedDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if err := s.OpenPath(dir); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// thumbCache keeps thumbnails in memory, keyed by path, size and modification
// time; it is emptied when it grows past its limit.
type thumbCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newThumbCache() *thumbCache { return &thumbCache{m: map[string][]byte{}} }

func (c *thumbCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[key]
	return b, ok
}

func (c *thumbCache) put(key string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= 500 {
		clear(c.m)
	}
	c.m[key] = b
}
