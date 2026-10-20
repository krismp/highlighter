// Package ytdlp resolves YouTube videos to direct stream URLs via yt-dlp.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/krismp/highlighter/internal/proc"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotYouTube = errors.New("not a YouTube video URL")
	ErrNoVideo    = errors.New("there is no YouTube video at this link; check it and try again")
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
	Bin    string
	Locate func() string // when set, finds the binary on every run, so an updated yt-dlp is picked up
	Deno   string        // JavaScript runtime for YouTube's player challenges; "" lets yt-dlp search PATH
}

// Args is ResolveArgs plus the runtime location, when one is known.
func (c Client) Args(url string) []string {
	if c.Deno == "" {
		return ResolveArgs(url)
	}
	return append([]string{"--js-runtimes", "deno:" + c.Deno}, ResolveArgs(url)...)
}

func (c Client) Resolve(ctx context.Context, url string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	bin := c.Bin
	if c.Locate != nil {
		bin = c.Locate()
	}
	cmd := proc.Command(ctx, bin, c.Args(url)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("yt-dlp: %s", lastLine(stderr.String(), err))
	}
	return ParseInfo(out)
}

// OEmbedURL is YouTube's public oEmbed endpoint, which answers in a fraction
// of a second where yt-dlp needs several.
var OEmbedURL = "https://www.youtube.com/oembed"

// Title looks up a video's title through oEmbed, so a video can be shown
// before yt-dlp has resolved it. ErrNoVideo means YouTube says the video does
// not exist; any other error just means the title is not known yet.
func Title(ctx context.Context, canonical string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OEmbedURL+"?format=json&url="+url.QueryEscape(canonical), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound:
		return "", ErrNoVideo
	case resp.StatusCode != http.StatusOK: // private or not embeddable: yt-dlp will say more
		return "", fmt.Errorf("oembed: %s", resp.Status)
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("oembed: %w", err)
	}
	return strings.TrimSpace(body.Title), nil
}

func lastLine(s string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback.Error()
}
