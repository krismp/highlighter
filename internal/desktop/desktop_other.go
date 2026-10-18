//go:build !darwin && !windows

package desktop

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// videosDir reads XDG_VIDEOS_DIR from ~/.config/user-dirs.dirs.
func videosDir(home string) string {
	if f, err := os.Open(filepath.Join(home, ".config", "user-dirs.dirs")); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "XDG_VIDEOS_DIR="); ok {
				v = strings.ReplaceAll(strings.Trim(v, `"`), "$HOME", home)
				if filepath.IsAbs(v) && v != home {
					return v
				}
			}
		}
	}
	return filepath.Join(home, "Videos")
}

// Open opens a URL, folder or file with its default app.
func Open(target string) error { return exec.Command("xdg-open", target).Run() }

// Reveal opens the folder containing path; xdg-open cannot select a file.
func Reveal(path string) error { return Open(filepath.Dir(path)) }
