// Package server exposes the HTTP API and serves the embedded UI.
package server

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/timecode"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ytdlp"
)

type Deps struct {
	Store       *store.Store
	Jobs        *job.Manager
	Resolver    job.Resolver
	Encoder     ffmpeg.Encoder
	Health      func(context.Context) tools.Status
	UpdateYtDlp func(context.Context) (string, error)
	OpenPath    func(string) error
	NewID       func() string
	UI          fs.FS
	Port        int // listen port; 0 accepts any port (tests)
}

type server struct{ Deps }

func New(d Deps) http.Handler {
	s := &server{d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/tools/update-ytdlp", s.updateYtDlp)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.openVideo)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("PUT /api/sessions/{id}", s.putSession)
	mux.HandleFunc("POST /api/sessions/{id}/clips/parse", s.parseClips)
	mux.HandleFunc("POST /api/sessions/{id}/process", s.process)
	mux.HandleFunc("POST /api/sessions/{id}/retry/{clipId}", s.retry)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.events)
	mux.HandleFunc("POST /api/sessions/{id}/open-folder", s.openFolder)
	mux.HandleFunc("GET /api/sessions/{id}/files/{name}", s.file)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "not found") })
	mux.Handle("/", spa(d.UI))
	return hostGuard(d.Port, mux)
}

// hostGuard rejects foreign Host headers (DNS rebinding) and non-JSON writes
// to /api/ (cross-site simple-request CSRF).
func hostGuard(port int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host, port) {
			writeError(w, 403, "forbidden host")
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, 415, "Content-Type must be application/json")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedHost(hostport string, port int) bool {
	host, p, err := net.SplitHostPort(hostport)
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		return false
	}
	return port == 0 || p == strconv.Itoa(port)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, 400, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *server) settings(w http.ResponseWriter) (store.Settings, bool) {
	st, err := s.Store.LoadSettings()
	if err != nil {
		writeError(w, 500, err.Error())
		return st, false
	}
	return st, true
}

func (s *server) loadSession(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	sess, err := s.Store.LoadSession(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "session not found")
		return sess, false
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return sess, false
	}
	return sess, true
}

// saveAndRespond persists sess and writes its view.
func (s *server) saveAndRespond(w http.ResponseWriter, sess store.Session, wrap func(SessionView) any) {
	saved, err := s.Store.SaveSession(sess)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	st, ok := s.settings(w)
	if !ok {
		return
	}
	v := buildView(saved, st.OutputRoot)
	if wrap != nil {
		writeJSON(w, 200, wrap(v))
		return
	}
	writeJSON(w, 200, v)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Health(r.Context()))
}

func (s *server) updateYtDlp(w http.ResponseWriter, r *http.Request) {
	out, err := s.UpdateYtDlp(r.Context())
	if err != nil {
		writeError(w, 500, strings.TrimSpace(out+"\n"+err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"output": out})
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	if st, ok := s.settings(w); ok {
		writeJSON(w, 200, st)
	}
}

func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var st store.Settings
	if !readJSON(w, r, &st) {
		return
	}
	if rest, ok := strings.CutPrefix(st.OutputRoot, "~/"); ok {
		home, _ := os.UserHomeDir()
		st.OutputRoot = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(st.OutputRoot) {
		writeError(w, 400, "output folder must be an absolute path")
		return
	}
	if st.Concurrency < 1 || st.Concurrency > 8 {
		writeError(w, 400, "concurrency must be between 1 and 8")
		return
	}
	if err := s.Store.SaveSettings(st); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.Store.RecentSessions(20)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	out := make([]RecentSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, RecentSession{VideoID: sess.VideoID, Title: sess.Title, ClipCount: len(sess.Clips), UpdatedAt: sess.UpdatedAt})
	}
	writeJSON(w, 200, out)
}

func (s *server) openVideo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, canonical, err := ytdlp.ParseURL(body.URL)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	sess, err := s.Store.LoadSession(id)
	if err == nil {
		if st, ok := s.settings(w); ok {
			writeJSON(w, 200, buildView(sess, st.OutputRoot))
		}
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writeError(w, 500, err.Error())
		return
	}
	info, err := s.Resolver.Resolve(r.Context(), canonical)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	s.saveAndRespond(w, store.Session{
		VideoID: id, URL: canonical, Title: info.Title, Duration: info.Duration,
		Width: info.Width, Height: info.Height, Clips: []clip.Clip{},
	}, nil)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	if st, ok := s.settings(w); ok {
		writeJSON(w, 200, buildView(sess, st.OutputRoot))
	}
}

func (s *server) putSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		LeadIn float64     `json:"leadIn"`
		Clips  []clip.Clip `json:"clips"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.LeadIn < 0 || body.LeadIn > clip.MaxLeadIn {
		writeError(w, 400, fmt.Sprintf("lead-in must be between 0 and %gs", clip.MaxLeadIn))
		return
	}
	sess.Clips = clip.Reconcile(sess.Clips, body.Clips, body.LeadIn != sess.LeadIn, s.NewID)
	sess.LeadIn = body.LeadIn
	s.saveAndRespond(w, sess, nil)
}

func (s *server) parseClips(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	entries, lineErrs := timecode.ParseList(body.Text)
	incoming := slices.Clone(sess.Clips)
	for _, e := range entries {
		incoming = append(incoming, clip.Clip{ID: s.NewID(), Start: e.Start, Duration: clip.DefaultDuration, Label: e.Label})
	}
	sess.Clips = clip.Reconcile(sess.Clips, incoming, false, s.NewID)
	s.saveAndRespond(w, sess, func(v SessionView) any { return parseResponse{Session: v, Errors: lineErrs} })
}

func (s *server) process(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Mode     job.Mode `json:"mode"`
		ReelName string   `json:"reelName"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !body.Mode.Valid() {
		writeError(w, 400, `mode must be "clips", "reel" or "both"`)
		return
	}
	st, ok := s.settings(w)
	if !ok {
		return
	}
	v := buildView(sess, st.OutputRoot)
	if !v.CanProcess {
		writeError(w, 409, "approve every clip and fix errors before processing")
		return
	}
	reelFile := clip.ReelName(cmp.Or(strings.TrimSpace(body.ReelName), sess.Title)) + ".mp4"
	if body.Mode != job.ModeClips && slices.ContainsFunc(v.Clips, func(c ClipView) bool { return c.File == reelFile }) {
		writeError(w, 400, "the reel name "+reelFile+" is already used by a clip; choose another")
		return
	}
	if err := os.MkdirAll(v.OutputDir, 0o755); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	req := job.Request{SessionID: sess.VideoID, URL: sess.URL, OutputDir: v.OutputDir,
		Mode: body.Mode, ReelFile: reelFile, Encoder: s.Encoder, Concurrency: st.Concurrency}
	for _, c := range v.Clips {
		req.Tasks = append(req.Tasks, job.Task{ClipID: c.ID, Range: clip.Range{Start: c.EffStart, Duration: c.EffDuration}, File: c.File})
	}
	jv, err := s.Jobs.Start(req)
	if errors.Is(err, job.ErrBusy) {
		writeError(w, 409, err.Error())
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, jv)
}

func (s *server) retry(w http.ResponseWriter, r *http.Request) {
	jv, err := s.Jobs.Retry(r.PathValue("id"), r.PathValue("clipId"))
	switch {
	case errors.Is(err, job.ErrNotFound):
		writeError(w, 404, err.Error())
	case errors.Is(err, job.ErrNotRetryable):
		writeError(w, 409, err.Error())
	case err != nil:
		writeError(w, 500, err.Error())
	default:
		writeJSON(w, 200, jv)
	}
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}
	current, updates, cancel := s.Jobs.Subscribe(r.PathValue("id"))
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	send := func(v job.View) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	if current != nil {
		send(*current)
	} else {
		fmt.Fprint(w, ": no job yet\n\n")
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-updates:
			send(v)
		}
	}
}

// sessionDir is the running job's folder, or where a job would write.
func (s *server) sessionDir(w http.ResponseWriter, r *http.Request) (string, bool) {
	if jv, ok := s.Jobs.View(r.PathValue("id")); ok {
		return jv.OutputDir, true
	}
	sess, ok := s.loadSession(w, r)
	if !ok {
		return "", false
	}
	st, ok := s.settings(w)
	return outputDir(st.OutputRoot, sess.Title), ok
}

func (s *server) openFolder(w http.ResponseWriter, r *http.Request) {
	dir, ok := s.sessionDir(w, r)
	if !ok {
		return
	}
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

func (s *server) file(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != filepath.Base(name) || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".mp4") {
		writeError(w, 400, "invalid file name")
		return
	}
	dir, ok := s.sessionDir(w, r)
	if !ok {
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, name))
}

// spa serves built assets and falls back to index.html for client routes.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if _, err := fs.Stat(ui, p); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			http.Error(w, "UI not built: run `make build`", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}
