// Package clip holds the clip model and the rules that decide what gets cut.
package clip

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/krismp/highlighter/internal/timecode"
)

const (
	DefaultDuration = 10.0
	MinDuration     = 1.0
	MaxDuration     = 120.0
	MaxLeadIn       = 30.0
)

// Clip is one play the user wants cut out of the video.
type Clip struct {
	ID       string  `json:"id"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Label    string  `json:"label"`
	Approved bool    `json:"approved"`
}

// Range is the section of the video actually cut.
type Range struct {
	Start, Duration float64
}

// Effective applies the lead-in (extending the clip backwards) and trims the
// end to the video duration when it is known (> 0).
func Effective(c Clip, leadIn, videoDuration float64) Range {
	start := max(0, c.Start-leadIn)
	end := c.Start + c.Duration
	if videoDuration > 0 {
		end = min(end, videoDuration)
	}
	return Range{Start: start, Duration: max(0, end-start)}
}

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Issue struct {
	ClipID   string   `json:"clipId"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Validate reports problems with clips, which must be sorted by Start.
// Errors block processing; warnings do not.
func Validate(clips []Clip, leadIn, videoDuration float64) []Issue {
	issues := []Issue{}
	add := func(c Clip, sev Severity, format string, a ...any) {
		issues = append(issues, Issue{ClipID: c.ID, Severity: sev, Message: fmt.Sprintf(format, a...)})
	}
	for i, c := range clips {
		if c.Duration < MinDuration || c.Duration > MaxDuration {
			add(c, SeverityError, "duration must be between %gs and %gs", MinDuration, MaxDuration)
		}
		if c.Start < 0 {
			add(c, SeverityError, "start must not be negative")
		}
		if videoDuration > 0 && c.Start >= videoDuration {
			add(c, SeverityError, "starts after the video ends (%s)", timecode.Format(videoDuration))
		} else if videoDuration > 0 && c.Start+c.Duration > videoDuration {
			add(c, SeverityWarning, "will be trimmed to end at %s", timecode.Format(videoDuration))
		}
		r := Effective(c, leadIn, videoDuration)
		for j := 0; j < i; j++ {
			o := clips[j]
			if math.Abs(o.Start-c.Start) < 0.05 {
				add(c, SeverityWarning, "same start as clip #%d", j+1)
				break
			}
			or := Effective(o, leadIn, videoDuration)
			if r.Start < or.Start+or.Duration && or.Start < r.Start+r.Duration {
				add(c, SeverityWarning, "overlaps clip #%d", j+1)
				break
			}
		}
	}
	return issues
}

// CanProcess is true when there is at least one clip, every clip is approved
// and no issue is an error.
func CanProcess(clips []Clip, issues []Issue) bool {
	if len(clips) == 0 {
		return false
	}
	for _, c := range clips {
		if !c.Approved {
			return false
		}
	}
	for _, is := range issues {
		if is.Severity == SeverityError {
			return false
		}
	}
	return true
}

// Reconcile merges an edited clip list into the stored one. New clips get an
// ID; approval is cleared for new clips, for clips whose start or duration
// changed, and for every clip when the lead-in changed. Result is sorted by Start.
func Reconcile(old, incoming []Clip, leadInChanged bool, newID func() string) []Clip {
	prev := make(map[string]Clip, len(old))
	for _, c := range old {
		prev[c.ID] = c
	}
	out := make([]Clip, 0, len(incoming))
	for _, c := range incoming {
		p, known := prev[c.ID]
		switch {
		case c.ID == "":
			c.ID = newID()
			c.Approved = false
		case !known, leadInChanged, p.Start != c.Start, p.Duration != c.Duration:
			c.Approved = false
		}
		c.Label = strings.TrimSpace(c.Label)
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b Clip) int { return cmp.Compare(a.Start, b.Start) })
	return out
}
