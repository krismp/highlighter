package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTemporary(t *testing.T) {
	home, _ := os.UserHomeDir()
	temp := []string{os.TempDir(), filepath.Join(os.TempDir(), "x", "Highlights")}
	if filepath.Separator == '/' {
		temp = append(temp, "/tmp/Highlights", "/private/var/folders/v7/abc/T/out")
	}
	for _, p := range temp {
		if !IsTemporary(p) {
			t.Errorf("IsTemporary(%q) = false; want true", p)
		}
	}
	for _, p := range []string{VideosDir(home), filepath.Join(VideosDir(home), "Highlights"), filepath.Join(home, "Desktop")} {
		if IsTemporary(p) {
			t.Errorf("IsTemporary(%q) = true; want false", p)
		}
	}
}

func TestVideosDirIsInHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := VideosDir(home); !filepath.IsAbs(got) || got == home {
		t.Errorf("VideosDir = %q", got)
	}
}
