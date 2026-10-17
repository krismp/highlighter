package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/krismp/highlighter/internal/desktop"
)

// inAppBundle reports whether this is Highlighter.app's executable, which
// Finder starts without a terminal.
func inAppBundle() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

// startBackground re-runs this executable as a detached server that logs to
// ~/Library/Logs/Highlighter.log, waits for it to answer, then returns so the
// app quits at once (the server keeps running until Quit is clicked in the UI).
// Launching the app again just opens the browser on the running server.
func startBackground(url string) {
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, "Library", "Logs", "Highlighter.log")
	fail := func(msg string) {
		log.Print(msg)
		desktop.Alert("Highlighter could not start", msg+"\n\nDetails are in "+logPath)
	}
	exe, err := os.Executable()
	if err != nil {
		fail(err.Error())
		return
	}
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fail(err.Error())
		return
	}
	defer logFile.Close()

	cmd := exec.Command(exe, append(os.Args[1:], "-foreground")...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fail(err.Error())
		return
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case err := <-exited:
			if running(url) { // another launch won the race for the port
				return
			}
			fail("The server stopped right after starting (" + errString(err) + ").")
			return
		case <-deadline:
			fail("The server did not respond within a minute.")
			return
		case <-time.After(250 * time.Millisecond):
			if running(url) {
				return
			}
		}
	}
}

func errString(err error) string {
	if err == nil {
		return "exit status 0"
	}
	return err.Error()
}
