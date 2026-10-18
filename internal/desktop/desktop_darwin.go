package desktop

import "os/exec"

const videosFolder = "Movies"

// Open opens a URL, folder or file with its default app.
func Open(target string) error { return exec.Command("open", target).Run() }

// Reveal shows a file selected in Finder.
func Reveal(path string) error { return exec.Command("open", "-R", path).Run() }
