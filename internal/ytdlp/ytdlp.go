// Package ytdlp resolves YouTube videos to direct stream URLs via yt-dlp.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotYouTube = errors.New("not a YouTube video URL")
	ErrLive       = errors.New("live and upcoming streams are not supported")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ParseURL extracts the video ID and returns the canonical watch URL.
func ParseURL(raw string) (id, canonical string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", ErrNotYouTube
	}
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "m.youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
			break
		}
		for _, prefix := range []string{"/shorts/", "/live/", "/embed/"} {
			if rest, ok := strings.CutPrefix(u.Path, prefix); ok {
				id = strings.Trim(rest, "/")
			}
		}
	}
	if !idPattern.MatchString(id) {
		return "", "", ErrNotYouTube
	}
	return id, "https://www.youtube.com/watch?v=" + id, nil
}

const FormatSelector = "bv*[height<=1080]+ba/b[height<=1080]/b"

func ResolveArgs(url string) []string {
	return []string{"-J", "--no-playlist", "--no-warnings", "-f", FormatSelector, "-S", "vcodec:h264", url}
}

type Stream struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

type Info struct {
	ID       string
	Title    string
	Duration float64
	Width    int
	Height   int
	Video    Stream
	Audio    *Stream // nil when Video is a muxed audio+video stream
}

type rawFormat struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"http_headers"`
	VCodec  string            `json:"vcodec"`
	ACodec  string            `json:"acodec"`
}

type rawInfo struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Duration         float64           `json:"duration"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	LiveStatus       string            `json:"live_status"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"http_headers"`
	RequestedFormats []rawFormat       `json:"requested_formats"`
}

// ParseInfo reads `yt-dlp -J` output.
func ParseInfo(data []byte) (Info, error) {
	var raw rawInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		return Info{}, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	if raw.LiveStatus == "is_live" || raw.LiveStatus == "is_upcoming" {
		return Info{}, ErrLive
	}
	info := Info{ID: raw.ID, Title: raw.Title, Duration: raw.Duration, Width: raw.Width, Height: raw.Height}
	if len(raw.RequestedFormats) > 0 {
		for _, f := range raw.RequestedFormats {
			s := Stream{URL: f.URL, Headers: f.Headers}
			switch {
			case f.VCodec != "" && f.VCodec != "none":
				info.Video = s
			case f.ACodec != "" && f.ACodec != "none":
				info.Audio = &s
			}
		}
	} else if raw.URL != "" {
		info.Video = Stream{URL: raw.URL, Headers: raw.Headers}
	}
	if info.Video.URL == "" {
		return Info{}, errors.New("yt-dlp returned no playable video stream")
	}
	return info, nil
}

// Client runs the yt-dlp binary.
type Client struct {
	Bin string
}

func (c Client) Resolve(ctx context.Context, url string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, ResolveArgs(url)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("yt-dlp: %s", lastLine(stderr.String(), err))
	}
	return ParseInfo(out)
}

func lastLine(s string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback.Error()
}
