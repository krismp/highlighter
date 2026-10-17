//go:build !darwin

package main

// Outside macOS the server always runs in the foreground: on Windows that is
// the console window double-clicking the .exe opens.
func inAppBundle() bool { return false }

func startBackground(string) {}
