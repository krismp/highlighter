package desktop

import "os/exec"

const videosFolder = "Movies"

// Open opens a URL, folder or file with its default app.
func Open(target string) error { return exec.Command("open", target).Run() }

// Reveal shows a file selected in Finder.
func Reveal(path string) error { return exec.Command("open", "-R", path).Run() }

// Alert shows a modal error dialog; used when there is no terminal to log to.
func Alert(title, message string) {
	_ = exec.Command("osascript", "-e", `on run argv
display alert (item 1 of argv) message (item 2 of argv) as critical
end run`, title, message).Run()
}
