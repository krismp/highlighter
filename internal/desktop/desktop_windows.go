package desktop

import (
	"os/exec"
	"syscall"
)

const videosFolder = "Videos"

// Open opens a URL, folder or file with its default app.
func Open(target string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Run()
}

// Reveal shows a file selected in File Explorer. explorer.exe needs the path
// quoted inside the /select, argument, so the command line is built by hand;
// it also exits non-zero on success, so its status is ignored.
func Reveal(path string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	_ = cmd.Run()
	return nil
}

// Alert is a no-op: on Windows the app runs in a console window that shows the log.
func Alert(title, message string) {}
