// Command highlighter serves the clipper UI on localhost.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/server"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ui"
	"github.com/krismp/highlighter/internal/ytdlp"
)

func main() {
	port := flag.Int("port", 7777, "port to listen on (127.0.0.1 only)")
	noOpen := flag.Bool("no-open", false, "do not open the browser on start")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.New(filepath.Join(home, ".config", "highlighter"), store.DefaultSettings(home))
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	status := tools.Check(ctx)
	if !status.YtDlp.Found || !status.FFmpeg.Found {
		log.Println("warning: yt-dlp and ffmpeg are required — run: brew install yt-dlp ffmpeg")
	}
	encoder := ffmpeg.X264
	if status.FFmpeg.Found {
		encoder = ffmpeg.DetectEncoder(ctx, status.FFmpeg.Path)
	}
	log.Printf("video encoder: %s", encoder)

	resolver := ytdlp.Client{Bin: cmp.Or(status.YtDlp.Path, "yt-dlp")}
	cutter := ffmpeg.Runner{Bin: cmp.Or(status.FFmpeg.Path, "ffmpeg")}
	handler := server.New(server.Deps{
		Store:    st,
		Jobs:     job.NewManager(resolver, cutter),
		Resolver: resolver,
		Encoder:  encoder,
		Health: func(ctx context.Context) tools.Status {
			s := tools.Check(ctx)
			s.Encoder = string(encoder)
			return s
		},
		UpdateYtDlp: tools.UpdateYtDlp,
		OpenPath:    func(p string) error { return exec.Command("open", p).Run() },
		NewID:       newID,
		UI:          ui.FS(),
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + addr
	log.Printf("highlighter running at %s", url)
	if !*noOpen {
		_ = exec.Command("open", url).Start()
	}
	log.Fatal(http.Serve(ln, handler))
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
