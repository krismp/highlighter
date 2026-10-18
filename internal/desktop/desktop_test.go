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

// TestTrash moves a real file to the user's Trash / Recycle Bin, so it only
// runs on CI (or with HIGHLIGHTER_TEST_TRASH=1) to keep developers' Trash clean.
func TestTrash(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getenv("HIGHLIGHTER_TEST_TRASH") == "" {
		t.Skip("moves a file into the real Trash; set HIGHLIGHTER_TEST_TRASH=1 to run")
	}
	home, _ := os.UserHomeDir()
	dir, err := os.MkdirTemp(home, "highlighter-trash-test-") // same disk as the Trash
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "01_0-02_trash-test.mp4")
	if err := os.WriteFile(p, []byte("clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Trash(p); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file still there after Trash (stat err %v)", err)
	}
}
