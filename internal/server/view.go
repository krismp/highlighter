package server

import (
	"path/filepath"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/timecode"
)

type ClipView struct {
	clip.Clip
	Index       int     `json:"index"`
	StartText   string  `json:"startText"`
	EffStart    float64 `json:"effStart"`
	EffDuration float64 `json:"effDuration"`
	File        string  `json:"file"`
}

type SessionView struct {
	VideoID      string       `json:"videoId"`
	URL          string       `json:"url"`
	Title        string       `json:"title"`
	Duration     float64      `json:"duration"`
	DurationText string       `json:"durationText"`
	LeadIn       float64      `json:"leadIn"`
	Clips        []ClipView   `json:"clips"`
	Issues       []clip.Issue `json:"issues"`
	CanProcess   bool         `json:"canProcess"`
	OutputDir    string       `json:"outputDir"`
	ReelName     string       `json:"reelName"` // default combined-reel name, without .mp4
}

type RecentSession struct {
	VideoID   string    `json:"videoId"`
	Title     string    `json:"title"`
	ClipCount int       `json:"clipCount"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type parseResponse struct {
	Session SessionView          `json:"session"`
	Errors  []timecode.LineError `json:"errors"`
}

func outputDir(root, title string) string {
	return filepath.Join(root, clip.SafeDirName(title))
}

func buildView(s store.Session, outputRoot string) SessionView {
	issues := clip.Validate(s.Clips, s.LeadIn, s.Duration)
	v := SessionView{
		VideoID: s.VideoID, URL: s.URL, Title: s.Title,
		Duration: s.Duration, DurationText: timecode.Format(s.Duration), LeadIn: s.LeadIn,
		Clips: make([]ClipView, 0, len(s.Clips)), Issues: issues,
		CanProcess: clip.CanProcess(s.Clips, issues),
		OutputDir:  outputDir(outputRoot, s.Title),
		ReelName:   clip.ReelName(s.Title),
	}
	for i, c := range s.Clips {
		r := clip.Effective(c, s.LeadIn, s.Duration)
		v.Clips = append(v.Clips, ClipView{
			Clip: c, Index: i + 1, StartText: timecode.Format(c.Start),
			EffStart: r.Start, EffDuration: r.Duration, File: clip.Filename(i+1, c),
		})
	}
	return v
}
