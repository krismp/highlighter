// Package tools discovers the external binaries highlighter depends on.
package tools

import (
	"context"
	"os/exec"
	"strings"
)

type ToolStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Status struct {
	YtDlp   ToolStatus `json:"ytDlp"`
	FFmpeg  ToolStatus `json:"ffmpeg"`
	Encoder string     `json:"encoder"`
}

func Check(ctx context.Context) Status {
	return Status{
		YtDlp:  probe(ctx, "yt-dlp", "--version"),
		FFmpeg: probe(ctx, "ffmpeg", "-version"),
	}
}

func probe(ctx context.Context, name string, versionArgs ...string) ToolStatus {
	path, err := exec.LookPath(name)
	if err != nil {
		return ToolStatus{}
	}
	out, _ := exec.CommandContext(ctx, path, versionArgs...).Output()
	return ToolStatus{Found: true, Path: path, Version: FirstLine(string(out))}
}

func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// UpdateYtDlp upgrades the Homebrew-installed yt-dlp.
func UpdateYtDlp(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "brew", "upgrade", "yt-dlp").CombinedOutput()
	return string(out), err
}
