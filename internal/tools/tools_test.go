package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"2026.09.01\n":                         "2026.09.01",
		"ffmpeg version 8.0 Copyright\nbuilt…": "ffmpeg version 8.0 Copyright",
		"":                                     "",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestFindPrefersEarlierDirs(t *testing.T) {
	user, bundled := t.TempDir(), t.TempDir()
	touch := func(dir string) string {
		p := filepath.Join(dir, exeName("highlighter-test-tool"))
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	l := Locator{UserDir: user, Dirs: []string{user, bundled}}
	if got := l.Find("highlighter-test-tool"); got != "" {
		t.Fatalf("Find with no tool = %q; want empty", got)
	}
	want := touch(bundled)
	if got := l.Find("highlighter-test-tool"); got != want {
		t.Fatalf("Find = %q; want bundled %q", got, want)
	}
	want = touch(user)
	if got := l.Find("highlighter-test-tool"); got != want {
		t.Fatalf("Find = %q; want user copy %q", got, want)
	}
}

func TestReleaseAsset(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  "yt-dlp_macos",
		{"darwin", "amd64"}:  "yt-dlp_macos",
		{"windows", "amd64"}: "yt-dlp.exe",
		{"windows", "arm64"}: "yt-dlp_arm64.exe",
		{"linux", "amd64"}:   "yt-dlp_linux",
	}
	for in, want := range cases {
		if got, err := ReleaseAsset(in[0], in[1]); err != nil || got != want {
			t.Errorf("ReleaseAsset(%s, %s) = %q, %v; want %q", in[0], in[1], got, err, want)
		}
	}
	if _, err := ReleaseAsset("plan9", "386"); err == nil {
		t.Error("ReleaseAsset(plan9) succeeded; want error")
	}
}

func TestIsHomebrew(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew paths are macOS/Linux only")
	}
	if !IsHomebrew("/opt/homebrew/bin/yt-dlp") {
		t.Error("/opt/homebrew/bin/yt-dlp should be Homebrew")
	}
	if IsHomebrew("/Applications/Highlighter.app/Contents/MacOS/yt-dlp") {
		t.Error("bundled yt-dlp should not be Homebrew")
	}
}
