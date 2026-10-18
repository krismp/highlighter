// Package tools discovers the external binaries highlighter depends on.
//
// The desktop app ships yt-dlp, ffmpeg and deno next to the server binary
// (Tauri sidecars) and the CA bundle in its resources folder. A yt-dlp
// downloaded by UpdateYtDlp lives in the user's config folder and wins over
// the bundled one; PATH is the last resort (Homebrew, development).
package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ToolStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Status struct {
	YtDlp   ToolStatus `json:"ytDlp"`
	FFmpeg  ToolStatus `json:"ffmpeg"`
	Deno    ToolStatus `json:"deno"`
	Encoder string     `json:"encoder"`
	OS      string     `json:"os"` // runtime.GOOS, so the UI can name the file manager
}

// Locator finds tools in Dirs (in order), then on PATH.
type Locator struct {
	UserDir string // writable; UpdateYtDlp downloads here
	Dirs    []string
}

// NewLocator searches userDir, then the executable's folder (desktop app
// sidecars), then the macOS app's Resources folder.
func NewLocator(userDir string) Locator {
	l := Locator{UserDir: userDir, Dirs: []string{userDir}}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dir := filepath.Dir(exe)
			l.Dirs = append(l.Dirs, dir, filepath.Join(dir, "..", "Resources"))
		}
	}
	return l
}

// Find returns the path of the named tool, or "" when it is nowhere.
func (l Locator) Find(name string) string {
	if p := l.File(exeName(name)); p != "" {
		return p
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

// File returns the first Dirs entry holding file (a data file such as the CA
// bundle, so PATH is not searched), or "".
func (l Locator) File(file string) string {
	for _, d := range l.Dirs {
		p := filepath.Join(d, file)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func (l Locator) Check(ctx context.Context) Status {
	return Status{
		YtDlp:  probe(ctx, l.Find("yt-dlp"), "--version"),
		FFmpeg: probe(ctx, l.Find("ffmpeg"), "-version"),
		Deno:   probe(ctx, l.Find("deno"), "--version"),
		OS:     runtime.GOOS,
	}
}

func probe(ctx context.Context, path string, versionArgs ...string) ToolStatus {
	if path == "" {
		return ToolStatus{}
	}
	out, _ := exec.CommandContext(ctx, path, versionArgs...).Output()
	return ToolStatus{Found: true, Path: path, Version: FirstLine(string(out))}
}

func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// IsHomebrew reports whether path is a Homebrew-managed binary.
func IsHomebrew(path string) bool {
	return strings.HasPrefix(path, "/opt/homebrew/") || strings.HasPrefix(path, "/usr/local/Cellar/") ||
		strings.HasPrefix(path, "/usr/local/bin/") && isSymlinkInto(path, "/Cellar/")
}

func isSymlinkInto(path, part string) bool {
	target, err := filepath.EvalSymlinks(path)
	return err == nil && strings.Contains(target, part)
}

const releases = "https://github.com/yt-dlp/yt-dlp/releases"

// ReleaseAsset is the standalone yt-dlp build for this OS and architecture.
func ReleaseAsset(goos, goarch string) (string, error) {
	switch {
	case goos == "darwin":
		return "yt-dlp_macos", nil // universal binary
	case goos == "windows" && goarch == "arm64":
		return "yt-dlp_arm64.exe", nil
	case goos == "windows":
		return "yt-dlp.exe", nil
	case goos == "linux" && goarch == "arm64":
		return "yt-dlp_linux_aarch64", nil
	case goos == "linux" && goarch == "amd64":
		return "yt-dlp_linux", nil
	}
	return "", fmt.Errorf("no yt-dlp build for %s/%s", goos, goarch)
}

// UpdateYtDlp brings yt-dlp up to date: through Homebrew when that is where
// it comes from, otherwise by downloading the latest release into UserDir.
func (l Locator) UpdateYtDlp(ctx context.Context) (string, error) {
	current := l.Find("yt-dlp")
	if IsHomebrew(current) {
		out, err := exec.CommandContext(ctx, "brew", "upgrade", "yt-dlp").CombinedOutput()
		return string(out), err
	}
	latest, err := latestTag(ctx)
	if err != nil {
		return "", err
	}
	if current != "" && probe(ctx, current, "--version").Version == latest {
		return "yt-dlp " + latest + " is the latest version", nil
	}
	asset, err := ReleaseAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(l.UserDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(l.UserDir, exeName("yt-dlp"))
	if err := download(ctx, releases+"/download/"+latest+"/"+asset, dest); err != nil {
		return "", err
	}
	got := probe(ctx, dest, "--version").Version
	if got == "" {
		os.Remove(dest)
		return "", errors.New("downloaded yt-dlp does not run")
	}
	return "Updated yt-dlp to " + got, nil
}

// latestTag reads the tag GitHub redirects /releases/latest to.
func latestTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, releases+"/latest", nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("check latest yt-dlp: %w", err)
	}
	resp.Body.Close()
	_, tag, ok := strings.Cut(resp.Header.Get("Location"), "/releases/tag/")
	if !ok || tag == "" {
		return "", fmt.Errorf("check latest yt-dlp: unexpected response %s", resp.Status)
	}
	return tag, nil
}

// download writes url to dest via a temp file, so a failed download never
// replaces a working binary.
func download(ctx context.Context, url, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download yt-dlp: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download yt-dlp: %s", resp.Status)
	}
	tmp := dest + ".download"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("download yt-dlp: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}
