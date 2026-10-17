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
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
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
	foreground := flag.Bool("foreground", false, "serve from this process even when started from the macOS app")
	flag.Parse()

	url := fmt.Sprintf("http://127.0.0.1:%d", *port)
	if running(url) {
		log.Printf("highlighter is already running at %s", url)
		if !*noOpen {
			_ = desktop.Open(url)
		}
		return
	}
	if !*foreground && inAppBundle() {
		startBackground(url)
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
	health := &healthCache{status: status}
	cutter := ffmpeg.Runner{Bin: cmp.Or(status.FFmpeg.Path, "ffmpeg"), CAFile: loc.File("cacert.pem")}
	handler := server.New(server.Deps{
		Store:       st,
		Jobs:        job.NewManager(resolver, cutter),
		Resolver:    resolver,
		Encoder:     encoder,
		Health:      health.get,
		UpdateYtDlp: health.afterUpdate(loc),
		OpenPath:    desktop.Open,
		RevealPath:  desktop.Reveal,
		Quit: func() {
			log.Println("quit requested from the app")
			os.Exit(0)
		},
		NewID: newID,
		UI:    ui.FS(),
		Port:  *port,
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("highlighter running at %s", url)
	log.Printf("keep this running while you use the app; click Quit in the app (or close this window) to stop")
	if !*noOpen {
		_ = desktop.Open(url)
	}
	log.Fatal(http.Serve(ln, handler))
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
		s.Encoder = h.status.Encoder
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
