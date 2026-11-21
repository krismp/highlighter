package server

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/job"
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
// are done if their files are in outputDir or every clip is approved
// (processing requires that, and any timing change resets it).
func videoStatus(sess store.Session, running bool, outputDir string) Status {
	switch {
	case running:
		return StatusProcessing
	case strings.TrimSpace(sess.Draft) != "":
		return StatusDraft
	case len(sess.Clips) == 0:
		return StatusEmpty
	case sess.ChangedAt.IsZero() && sess.ProcessedAt.IsZero():
		if allApproved(sess) || outputsExist(sess, outputDir) {
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

// withJobOutcome counts a job that has succeeded but whose ProcessedAt is not
// saved yet (the gap before markProcessed runs) as processing the session,
// unless the clips changed since the job started.
func withJobOutcome(sess store.Session, jv job.View) store.Session {
	if !jv.Running && jv.Succeeded() && !jv.StartedAt.Before(sess.ChangedAt) && jv.StartedAt.After(sess.ProcessedAt) {
		sess.ProcessedAt = jv.StartedAt
	}
	return sess
}

func allApproved(sess store.Session) bool {
	return !slices.ContainsFunc(sess.Clips, func(c clip.Clip) bool { return !c.Approved })
}
