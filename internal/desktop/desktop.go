// Package desktop opens URLs, folders and files with the operating system's
// default handlers, and knows where the OS keeps the user's videos.
package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VideosDir is the user's own videos folder, where average users look for
// their clips: Movies on macOS, the Videos known folder on Windows (wherever
// it has been moved, e.g. into OneDrive), XDG_VIDEOS_DIR or ~/Videos on Linux.
func VideosDir(home string) string {
	return videosDir(home)
}

// IsTemporary reports whether path is inside a temporary folder, which the
// system empties and most people cannot find.
func IsTemporary(path string) bool {
	path = filepath.Clean(path)
	roots := []string{os.TempDir(), "/tmp", "/private/tmp", "/var/tmp", "/var/folders", "/private/var/folders"}
	if real, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		roots = append(roots, real)
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if path == root || strings.HasPrefix(strings.ToLower(path), strings.ToLower(root)+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// uniqueRename moves path into dir, adding " 2", " 3"... before the
// extension when a file of that name is already there.
func uniqueRename(path, dir string) (string, error) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s %d%s", stem, i, ext)
		}
		dest := filepath.Join(dir, name)
		if _, err := os.Lstat(dest); err == nil {
			continue
		}
		if err := os.Rename(path, dest); err != nil {
			return "", fmt.Errorf("move to Trash: %w", err)
		}
		return name, nil
	}
}
