//go:build !darwin && !windows

package desktop

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

// Trash follows the freedesktop.org trash spec, so file managers can restore it.
func Trash(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	trash := filepath.Join(home, ".local", "share", "Trash")
	if err := os.MkdirAll(filepath.Join(trash, "info"), 0o700); err != nil {
		return err
	}
	name, err := uniqueRename(abs, filepath.Join(trash, "files"))
	if err != nil {
		return err
	}
	info := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		(&url.URL{Path: abs}).EscapedPath(), time.Now().Format("2006-01-02T15:04:05"))
	return os.WriteFile(filepath.Join(trash, "info", name+".trashinfo"), []byte(info), 0o600)
}
