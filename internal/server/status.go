package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/store"
)

// Status is where a video stands in the Videos list.
type Status string

const (
	StatusProcessing Status = "processing"
	StatusDraft      Status = "draft"
	StatusDone       Status = "done"
	StatusEmpty      Status = "empty"
)

// videoStatus: a draft has clips changed since the last successful run, or
// unparsed paste text. Sessions saved before ChangedAt/ProcessedAt existed
// are judged by their files in outputDir.
func videoStatus(sess store.Session, running bool, outputDir string) Status {
	switch {
	case running:
		return StatusProcessing
	case strings.TrimSpace(sess.Draft) != "":
		return StatusDraft
	case len(sess.Clips) == 0:
		return StatusEmpty
	case sess.ChangedAt.IsZero() && sess.ProcessedAt.IsZero():
		if outputsExist(sess, outputDir) {
			return StatusDone
		}
		return StatusDraft
	case !sess.ProcessedAt.IsZero() && !sess.ProcessedAt.Before(sess.ChangedAt):
		return StatusDone
	}
	return StatusDraft
}

// outputsExist reports whether every clip's file, or the combined reel, is in dir.
func outputsExist(sess store.Session, dir string) bool {
	if fileExists(filepath.Join(dir, clip.ReelName(sess.Title)+".mp4")) {
		return true
	}
	for i, c := range sess.Clips {
		if !fileExists(filepath.Join(dir, clip.Filename(i+1, c))) {
			return false
		}
	}
	return true
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
