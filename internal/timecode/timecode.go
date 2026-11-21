// Package timecode parses and formats video timestamps.
package timecode

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var pattern = regexp.MustCompile(`^\d{1,6}(?::\d{1,2}){0,2}(?:\.\d+)?$`)

// Parse converts "83", "1:23", "01:02:03" or "1:23.5" into seconds.
func Parse(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !pattern.MatchString(s) {
		return 0, fmt.Errorf("invalid timestamp %q (use 83, 1:23 or 1:02:03)", s)
	}
	whole, frac, _ := strings.Cut(s, ".")
	total := 0
	for i, p := range strings.Split(whole, ":") {
		n, _ := strconv.Atoi(p)
		if i > 0 && n >= 60 {
			return 0, fmt.Errorf("invalid timestamp %q: minutes and seconds must be below 60", s)
		}
		total = total*60 + n
	}
	sec := float64(total)
	if frac != "" {
		f, _ := strconv.ParseFloat("0."+frac, 64)
		sec += f
	}
	return sec, nil
}

// Format renders whole seconds as m:ss or h:mm:ss.
func Format(sec float64) string {
	t := int(max(sec, 0))
	h, m, s := t/3600, t%3600/60, t%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// Entry is one parsed line of a pasted timestamp list. Duration is set only
// when the line gave a "start - end" range; 0 means "use the default length".
type Entry struct {
	Start    float64
	Duration float64
	Label    string
}

// rangePattern matches "2:22 - 3:02 [label]". The end must contain a colon so
// "1:23 - 45 points" still reads as a start time followed by a label.
var rangePattern = regexp.MustCompile(
	`^(\d{1,6}(?::\d{1,2}){0,2}(?:\.\d+)?)\s*(?:-|–|—|~|\s+to\s+)\s*` +
		`(\d{1,6}(?::\d{1,2}){1,2}(?:\.\d+)?)(?:$|[\s|,;:\-–—]+(.*)$)`)

// LineError reports a line of a pasted list that could not be parsed.
type LineError struct {
	Line int    `json:"line"`
	Text string `json:"text"`
	Err  string `json:"err"`
}

const labelTrim = " \t-–—|:,;"

// ParseList parses one "<timestamp> [label]" or "<start> - <end> [label]" per line. A line made only of
// timestamps separated by commas, semicolons or spaces yields one entry each.
func ParseList(text string) ([]Entry, []LineError) {
	entries := []Entry{}
	errs := []LineError{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := rangePattern.FindStringSubmatch(line); m != nil {
			e, err := parseRange(m[1], m[2])
			if err != nil {
				errs = append(errs, LineError{Line: i + 1, Text: line, Err: err.Error()})
				continue
			}
			e.Label = strings.Trim(m[3], labelTrim)
			entries = append(entries, e)
			continue
		}
		if starts := allTimestamps(line); starts != nil {
			for _, s := range starts {
				entries = append(entries, Entry{Start: s})
			}
			continue
		}
		stamp, rest := line, ""
		if idx := strings.IndexFunc(line, unicode.IsSpace); idx >= 0 {
			stamp, rest = line[:idx], line[idx:]
		}
		start, err := Parse(strings.TrimRight(stamp, labelTrim)) // "3:47: 3PT", "3:47- 3PT"
		if err != nil {
			errs = append(errs, LineError{Line: i + 1, Text: line, Err: err.Error()})
			continue
		}
		entries = append(entries, Entry{Start: start, Label: strings.Trim(rest, labelTrim)})
	}
	return entries, errs
}

func parseRange(from, to string) (Entry, error) {
	start, err := Parse(from)
	if err != nil {
		return Entry{}, err
	}
	end, err := Parse(to)
	if err != nil {
		return Entry{}, err
	}
	if end <= start {
		return Entry{}, fmt.Errorf("range end %s must be after its start %s", to, from)
	}
	return Entry{Start: start, Duration: end - start}, nil
}

func allTimestamps(line string) []float64 {
	fields := strings.FieldsFunc(line, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})
	if len(fields) < 2 {
		return nil
	}
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		s, err := Parse(f)
		if err != nil {
			return nil
		}
		out = append(out, s)
	}
	return out
}
