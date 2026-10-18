// Command highlighter serves the clipper UI on localhost.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/krismp/highlighter/internal/desktop"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/server"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ui"
	"github.com/krismp/highlighter/internal/ytdlp"
)

var version = "dev" // set by scripts/package.sh

func main() {
	port := flag.Int("port", 7777, "port to listen on (127.0.0.1 only)")
	noOpen := flag.Bool("no-open", false, "do not open the browser on start")
	exitWithStdin := flag.Bool("exit-with-stdin", false, "shut down when stdin closes (the desktop app holds it open while it runs)")
	flag.Parse()

	url := fmt.Sprintf("http://127.0.0.1:%d", *port)
	if running(url) {
		log.Printf("highlighter is already running at %s", url)
		if !*noOpen {
			_ = desktop.Open(url)
		}
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	cfg := filepath.Join(home, ".config", "highlighter")
	st, err := store.New(cfg, store.DefaultSettings(desktop.VideosDir(home)))
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	loc := tools.NewLocator(filepath.Join(cfg, "bin"))
	status := loc.Check(ctx)
	if !status.YtDlp.Found || !status.FFmpeg.Found {
		log.Println("warning: yt-dlp and ffmpeg were not found — reinstall Highlighter, or install them and put them on PATH")
	}
	encoder := ffmpeg.X264
	if status.FFmpeg.Found {
		encoder = ffmpeg.DetectEncoder(ctx, status.FFmpeg.Path)
	}
	log.Printf("highlighter %s", version)
	log.Printf("yt-dlp: %s", cmp.Or(status.YtDlp.Path, "not found"))
	log.Printf("ffmpeg: %s", cmp.Or(status.FFmpeg.Path, "not found"))
	log.Printf("deno:   %s", cmp.Or(status.Deno.Path, "not found"))
	log.Printf("video encoder: %s", encoder)

	resolver := ytdlp.Client{Deno: status.Deno.Path, Locate: func() string { return cmp.Or(loc.Find("yt-dlp"), "yt-dlp") }}
	// Probing the tools takes seconds (yt-dlp is a self-extracting binary), so
	// the status is measured once and again only after updating yt-dlp.
	status.Encoder = string(encoder)
	status.Version = version
	health := &healthCache{status: status}
	cutter := ffmpeg.Runner{Bin: cmp.Or(status.FFmpeg.Path, "ffmpeg"), CAFile: loc.File("cacert.pem")}
	jobs := job.NewManager(resolver, cutter)
	shutdown := shutdownOnce(jobs)
	handler := server.New(server.Deps{
		Store:       st,
		Jobs:        jobs,
		Resolver:    resolver,
		Encoder:     encoder,
		Health:      health.get,
		UpdateYtDlp: health.afterUpdate(loc),
		OpenPath:    desktop.Open,
		RevealPath:  desktop.Reveal,
		TrashPath:   desktop.Trash,
		Internet:    tools.CheckInternet,
		Quit:        func() { shutdown("Quit clicked in the app") },
		NewID:       newID,
		UI:          ui.FS(),
		Port:        *port,
	})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { shutdown("received " + (<-signals).String()) }()
	if *exitWithStdin {
		// The desktop app holds our stdin open; it closes when the app exits
		// for any reason, crashes and force-quits included.
		go func() {
			io.Copy(io.Discard, os.Stdin)
			shutdown("the desktop app closed")
		}()
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("highlighter running at %s", url)
	if !*noOpen {
		_ = desktop.Open(url)
	}
	log.Fatal(http.Serve(ln, handler))
}

// shutdownOnce returns the single way the server stops: cancel all clip work
// (killing yt-dlp and ffmpeg with their children, removing partial files),
// wait briefly for it, then exit. Later calls wait for the first to finish.
func shutdownOnce(jobs *job.Manager) func(reason string) {
	var once sync.Once
	return func(reason string) {
		once.Do(func() {
			log.Printf("shutting down: %s", reason)
			if !jobs.Shutdown(10 * time.Second) {
				log.Println("some clip work did not stop within 10s; exiting anyway")
			}
			os.Exit(0)
		})
		select {} // another caller is exiting the process
	}
}

type healthCache struct {
	mu     sync.Mutex
	status tools.Status
}

func (h *healthCache) get(context.Context) tools.Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

func (h *healthCache) afterUpdate(loc tools.Locator) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		out, err := loc.UpdateYtDlp(ctx)
		s := loc.Check(ctx)
		h.mu.Lock()
		s.Encoder, s.Version = h.status.Encoder, h.status.Version
		h.status = s
		h.mu.Unlock()
		return out, err
	}
}

// running reports whether a highlighter already answers at url.
func running(url string) bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url + "/api/ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct{ App string }
	return resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil && body.App == "highlighter"
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
