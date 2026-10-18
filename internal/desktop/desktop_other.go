//go:build !darwin && !windows

package desktop

import (
	"os/exec"
	"path/filepath"
)

const videosFolder = "Videos"

// Open opens a URL, folder or file with its default app.
func Open(target string) error { return exec.Command("xdg-open", target).Run() }

// Reveal opens the folder containing path; xdg-open cannot select a file.
func Reveal(path string) error { return Open(filepath.Dir(path)) }
