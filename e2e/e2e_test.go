//go:build e2e

// Package e2e clips a real public video with the real yt-dlp and ffmpeg.
package e2e

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/ytdlp"
)

const url = "https://www.youtube.com/watch?v=jNQXAC9IVRw" // "Me at the zoo", 19 s

func TestClipRealVideo(t *testing.T) {
	ytBin, err1 := exec.LookPath("yt-dlp")
	ffBin, err2 := exec.LookPath("ffmpeg")
	if err1 != nil || err2 != nil {
		t.Skip("yt-dlp and ffmpeg must be installed")
	}
	ctx := context.Background()
	resolver := ytdlp.Client{Bin: ytBin}
	info, err := resolver.Resolve(ctx, url)
	if err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	m := job.NewManager(resolver, ffmpeg.Runner{Bin: ffBin})
	_, err = m.Start(job.Request{
		SessionID: "jNQXAC9IVRw", URL: url, OutputDir: out, Concurrency: 1,
		Encoder: ffmpeg.DetectEncoder(ctx, ffBin),
		Tasks:   []job.Task{{ClipID: "a", Range: clip.Range{Start: 5, Duration: 3}, File: "01_0-05.mp4"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		v, _ := m.View("jNQXAC9IVRw")
		if !v.Running {
			if v.Clips[0].Status != job.Done {
				t.Fatalf("clip failed: %+v", v.Clips[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(200 * time.Millisecond)
	}

	probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration", "-of", "json",
		filepath.Join(out, "01_0-05.mp4")).Output()
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Streams []struct{ Width, Height int }
		Format  struct{ Duration string }
	}
	json.Unmarshal(probe, &p)
	dur, _ := strconv.ParseFloat(p.Format.Duration, 64)
	if math.Abs(dur-3) > 0.25 {
		t.Errorf("duration = %.3f; want ≈3", dur)
	}
	if len(p.Streams) != 1 || p.Streams[0].Width != info.Width || p.Streams[0].Height != info.Height {
		t.Errorf("dimensions = %+v; want %dx%d (source)", p.Streams, info.Width, info.Height)
	}
}
