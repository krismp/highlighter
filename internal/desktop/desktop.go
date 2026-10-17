// Package desktop opens URLs, folders and files with the operating system's
// default handlers.
package desktop

import "path/filepath"

// VideosDir is where the OS keeps the user's videos: ~/Movies on macOS,
// ~/Videos elsewhere.
func VideosDir(home string) string {
	return filepath.Join(home, videosFolder)
}
