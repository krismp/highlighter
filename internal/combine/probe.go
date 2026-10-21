// Package combine joins local video files into one MP4: it probes each file
// with ffmpeg, plans the cheapest render that gives a clean result (stream
// copy when every file matches, otherwise one scale-pad-concat encode), and
// runs one combine at a time with live progress.
package combine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/krismp/highlighter/internal/proc"
)

// Media is what probing a file found out. Width and Height are as displayed,
// after the rotation phones record instead of turning the pixels.
type Media struct {
	Duration   float64 `json:"duration"` // seconds
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Rotation   int     `json:"rotation"` // degrees, 0/90/180/270
	FPS        float64 `json:"fps"`
	VideoCodec string  `json:"videoCodec"`
	PixFmt     string  `json:"pixFmt"`
	HasAudio   bool    `json:"hasAudio"`
	AudioCodec string  `json:"audioCodec"`
	SampleRate int     `json:"sampleRate"`
	Channels   string  `json:"channels"` // layout as ffmpeg names it: "stereo", "mono", "5.1(side)"
	Size       int64   `json:"size"`     // bytes
}

var (
	ErrNoVideo    = errors.New("this file has no video")
	ErrNoDuration = errors.New("can't tell how long this video is")

	durationRe = regexp.MustCompile(`Duration: (\d+):(\d{2}):(\d{2}(?:\.\d+)?)`)
	streamRe   = regexp.MustCompile(`^\s*Stream #\d+:\d+\S*: (Video|Audio|Subtitle|Data|Attachment): (\w+)(.*)$`)
	sizeRe     = regexp.MustCompile(`, (\d{2,5})x(\d{2,5})[ ,\[]`)
	fpsRe      = regexp.MustCompile(`, ([\d.]+) fps`)
	tbrRe      = regexp.MustCompile(`, ([\d.]+)(k?) tbr`)
	pixRe      = regexp.MustCompile(`^[^,]*, ([a-z0-9_]+)[(,]`)
	audioRe    = regexp.MustCompile(`, (\d+) Hz, ([^,]+),`)
	rotationRe = regexp.MustCompile(`(?i)rotation of (-?[\d.]+) degrees`)
)

// ParseProbe reads the stream summary `ffmpeg -i <file>` prints to stderr.
// It uses the first video stream (skipping cover art) and the first audio stream.
func ParseProbe(out string) (Media, error) {
	var m Media
	if d := durationRe.FindStringSubmatch(out); d != nil {
		h, _ := strconv.Atoi(d[1])
		mm, _ := strconv.Atoi(d[2])
		s, _ := strconv.ParseFloat(d[3], 64)
		m.Duration = float64(h*3600+mm*60) + s
	}
	video, audio := false, false
	inVideo := false // the lines after our video stream's header belong to it, up to the next stream
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		s := streamRe.FindStringSubmatch(line)
		if s == nil {
			if inVideo {
				if r := rotationRe.FindStringSubmatch(line); r != nil {
					deg, _ := strconv.ParseFloat(r[1], 64)
					m.Rotation = ((int(math.Round(deg)) % 360) + 360) % 360
				}
			}
			continue
		}
		inVideo = false
		kind, codec, rest := s[1], s[2], s[3]
		switch {
		case kind == "Video" && !video && !strings.Contains(rest, "attached pic"):
			video, inVideo = true, true
			m.VideoCodec = codec
			if p := pixRe.FindStringSubmatch(rest); p != nil {
				m.PixFmt = p[1]
			}
			if wh := sizeRe.FindStringSubmatch(rest + " "); wh != nil {
				m.Width, _ = strconv.Atoi(wh[1])
				m.Height, _ = strconv.Atoi(wh[2])
			}
			if f := fpsRe.FindStringSubmatch(rest); f != nil {
				m.FPS, _ = strconv.ParseFloat(f[1], 64)
			} else if f := tbrRe.FindStringSubmatch(rest); f != nil && f[2] == "" {
				m.FPS, _ = strconv.ParseFloat(f[1], 64)
			}
		case kind == "Audio" && !audio:
			audio = true
			m.HasAudio, m.AudioCodec = true, codec
			if a := audioRe.FindStringSubmatch(rest); a != nil {
				m.SampleRate, _ = strconv.Atoi(a[1])
				m.Channels = strings.TrimSpace(a[2])
			}
		}
	}
	if !video || m.Width == 0 || m.Height == 0 {
		return m, ErrNoVideo
	}
	if m.Rotation == 90 || m.Rotation == 270 {
		m.Width, m.Height = m.Height, m.Width
	}
	if m.Duration <= 0 {
		return m, ErrNoDuration
	}
	return m, nil
}

// Tool runs the ffmpeg binary to look into local video files.
type Tool struct{ Bin string }

// Probe describes the video file at path.
func (t Tool) Probe(ctx context.Context, path string) (Media, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Media{}, err
	}
	if fi.IsDir() {
		return Media{}, errors.New("this is a folder, not a video")
	}
	// ffmpeg exits non-zero without an output file; the summary is still printed.
	var stderr bytes.Buffer
	cmd := proc.Command(ctx, t.Bin, "-hide_banner", "-nostdin", "-i", path)
	cmd.Stderr = &stderr
	_ = cmd.Run()
	out := stderr.String()
	if !strings.Contains(out, "Input #0") {
		return Media{}, fmt.Errorf("not a video ffmpeg can read: %s", lastLine(out))
	}
	m, err := ParseProbe(out)
	m.Size = fi.Size()
	return m, err
}

// Thumbnail returns a small JPEG frame of the video at path, taken at second at.
func (t Tool) Thumbnail(ctx context.Context, path string, at float64) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := proc.Command(ctx, t.Bin, "-hide_banner", "-nostdin", "-loglevel", "error",
		"-ss", seconds(at), "-i", path, "-frames:v", "1",
		"-vf", "scale=360:360:force_original_aspect_ratio=decrease:force_divisible_by=2",
		"-q:v", "5", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || stdout.Len() == 0 {
		return nil, fmt.Errorf("thumbnail: %v %s", err, lastLine(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func seconds(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
