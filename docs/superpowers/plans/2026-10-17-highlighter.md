# highlighter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local, single-binary YouTube clipper. The user pastes a URL and play start timestamps, reviews each clip's duration in an embedded player, and gets one frame-accurate MP4 per play.

**Architecture:** A Go HTTP server (`127.0.0.1:7777`) embeds a Vite React SPA. Go coordinates two subprocesses: `yt-dlp -J` resolves the direct stream URLs, and `ffmpeg` input-seeks on those URLs to fetch only each clip's bytes and re-encode it (VideoToolbox, falling back to libx264). A job manager runs clips through a worker pool with retries and pushes progress to the browser over SSE. Sessions persist as JSON files.

**Tech Stack:** Go 1.26 (stdlib only: `net/http` patterns, `embed`, `os/exec`), yt-dlp + ffmpeg (Homebrew), Vite + React 19 + TypeScript, TanStack Router + TanStack Query, Tailwind CSS v4, shadcn/ui, pnpm.

**Spec:** `docs/superpowers/specs/2026-10-17-highlighter-design.md`

## Global Constraints

- Go module path: `github.com/krismp/highlighter`. Go ≥ 1.26 (installed: go1.26.4). No third-party Go dependencies.
- Node ≥ 24 (installed v24.18.0), pnpm ≥ 12 (installed 12.5.1).
- Binary name `highlighter`. Config dir `~/.config/highlighter`. Default output root `~/Movies/Highlights`.
- Server listens on `127.0.0.1` only. Default port `7777`.
- Clip duration: default `10`, min `1`, max `120` seconds. Lead-in `0`–`30` s, default `0`.
- Concurrency default `3`, allowed `1`–`8`. Automatic retries: `2` (backoff 2 s, 5 s).
- Format selector `bv*[height<=1080]+ba/b[height<=1080]/b` with `-S vcodec:h264`.
- Encoders: `h264_videotoolbox -b:v 8M -maxrate 12M -bufsize 16M`, fallback `libx264 -preset veryfast -crf 20`. Audio `aac 160k`. `-pix_fmt yuv420p -movflags +faststart`. No scaling or cropping.
- File name `NN_<m-ss>[_slug].mp4`. Combined reel `highlights_full.mp4`.
- Every JSON array in API responses must be `[]`, never `null`. Errors are `{"error": "..."}`.
- yt-dlp and ffmpeg are **not installed** on this machine yet. Task 8 installs them with `brew install yt-dlp ffmpeg`. Unit tests never need them.

## Review Focus

1. **Pasted timestamp lists that aren't neat one-per-line** (`1:23, 2:45; 3:10`, `12:34 - Andi`, CRLF line endings from Windows notes): all of them should parse into the expected entries. Pinned in Task 1 (`TestParseList`).
2. **The last play of the match, where start + 10 s runs past the video end**: this should be trimmed with a warning, not blocked and not failed in ffmpeg. Pinned in Task 2 (`TestValidate`, `TestEffective`).
3. **Stream URLs expiring or returning 403 between attempts**: every retry should re-resolve via yt-dlp. Pinned in Task 6 (`TestRetriesTransientFailuresAndReResolves`).
4. **Match titles containing `/`, `:`, emoji or 200 characters**: these should produce a safe, bounded folder name. Pinned in Task 2 (`TestSafeDirName`).
5. **URLs copied from the share sheet or the address bar** (`youtu.be/ID?si=…`, `watch?v=ID&t=754s&list=…`, no scheme, `m.youtube.com`): these should all resolve to the canonical URL. Pinned in Task 4 (`TestParseURL`).

---

## File Structure

```
go.mod
Makefile
README.md
.gitignore
cmd/highlighter/main.go           wiring, flags, browser open
internal/timecode/timecode.go     Parse, Format, ParseList
internal/clip/clip.go             Clip, Range, Effective, Validate, CanProcess, Reconcile
internal/clip/naming.go           Filename, Slug, SafeDirName
internal/store/store.go           Session/Settings JSON persistence
internal/ytdlp/ytdlp.go           ParseURL, ResolveArgs, ParseInfo, Client.Resolve
internal/ffmpeg/args.go           CutSpec, CutArgs, ConcatArgs, ConcatList, ProgressFraction
internal/ffmpeg/run.go            Runner.Cut/Concat, DetectEncoder
internal/tools/tools.go           binary discovery + yt-dlp update
internal/job/job.go               Manager: worker pool, retries, concat, subscribers
internal/server/server.go         routes + handlers
internal/server/view.go           SessionView/ClipView builder
internal/ui/ui.go                 go:embed of web build output
e2e/e2e_test.go                   real-tools test (build tag e2e)
web/                              Vite app (src/lib/api.ts, hooks/, components/, routes/)
```

---

### Task 1: Go module + timecode package

**Files:**
- Create: `go.mod`, `.gitignore`, `internal/timecode/timecode.go`
- Test: `internal/timecode/timecode_test.go`

**Interfaces:**
- Produces:
  - `func Parse(s string) (float64, error)`
  - `func Format(sec float64) string` — whole seconds, `m:ss` or `h:mm:ss`
  - `type Entry struct { Start float64; Label string }`
  - `type LineError struct { Line int \`json:"line"\`; Text string \`json:"text"\`; Err string \`json:"err"\` }`
  - `func ParseList(text string) ([]Entry, []LineError)` — never returns nil slices

- [ ] **Step 1: Initialise the module**

```bash
go mod init github.com/krismp/highlighter
```

Create `.gitignore`:

```gitignore
/bin/
internal/ui/dist/*
!internal/ui/dist/.gitkeep
web/node_modules/
.DS_Store
```

- [ ] **Step 2: Write the failing tests** — `internal/timecode/timecode_test.go`

```go
package timecode

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	ok := map[string]float64{
		"83":       83,
		"0":        0,
		"1:23":     83,
		"01:23":    83,
		"1:05":     65,
		"1:02:03":  3723,
		"01:02:03": 3723,
		"1:23.5":   83.5,
		" 2:00 ":   120,
		"90:00":    5400,
	}
	for in, want := range ok {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "1:60", "1:2:60", "1:2:3:4", "-5", "1e3", "1::2", ":30", "1:23:", "Inf"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded; want error", in)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := map[float64]string{0: "0:00", 5: "0:05", 83: "1:23", 83.9: "1:23", 3723: "1:02:03", -4: "0:00"}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%v) = %q; want %q", in, got, want)
		}
	}
}

func TestParseList(t *testing.T) {
	text := "12:34 Andi crossover\r\n\n  0:05 - fast break\nbanana\n1:23, 2:45; 3:10\n4:00 | and-one\n5:00\n"
	entries, errs := ParseList(text)
	want := []Entry{
		{754, "Andi crossover"},
		{5, "fast break"},
		{83, ""}, {165, ""}, {190, ""},
		{240, "and-one"},
		{300, ""},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v\nwant      %+v", entries, want)
	}
	if len(errs) != 1 || errs[0].Line != 4 || errs[0].Text != "banana" || errs[0].Err == "" {
		t.Errorf("errs = %+v; want one error for line 4 'banana'", errs)
	}
}

func TestParseListEmptyReturnsEmptySlices(t *testing.T) {
	entries, errs := ParseList("   \n")
	if entries == nil || errs == nil || len(entries) != 0 || len(errs) != 0 {
		t.Errorf("got %#v, %#v; want empty non-nil slices", entries, errs)
	}
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run: `go test ./internal/timecode/`
Expected: FAIL (`undefined: Parse`)

- [ ] **Step 4: Implement** — `internal/timecode/timecode.go`

```go
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

// Entry is one parsed line of a pasted timestamp list.
type Entry struct {
	Start float64
	Label string
}

// LineError reports a line of a pasted list that could not be parsed.
type LineError struct {
	Line int    `json:"line"`
	Text string `json:"text"`
	Err  string `json:"err"`
}

const labelTrim = " \t-–—|:,;"

// ParseList parses one "<timestamp> [label]" per line. A line made only of
// timestamps separated by commas, semicolons or spaces yields one entry each.
func ParseList(text string) ([]Entry, []LineError) {
	entries := []Entry{}
	errs := []LineError{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
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
		start, err := Parse(stamp)
		if err != nil {
			errs = append(errs, LineError{Line: i + 1, Text: line, Err: err.Error()})
			continue
		}
		entries = append(entries, Entry{Start: start, Label: strings.Trim(rest, labelTrim)})
	}
	return entries, errs
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
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./internal/timecode/`
Expected: `ok`

- [ ] **Step 6: Commit**

```bash
git add go.mod .gitignore internal/timecode
git commit -m "feat(timecode): parse and format timestamps and pasted lists"
```

---

### Task 2: clip package — range math, validation, approval, naming

**Files:**
- Create: `internal/clip/clip.go`, `internal/clip/naming.go`
- Test: `internal/clip/clip_test.go`, `internal/clip/naming_test.go`

**Interfaces:**
- Consumes: `timecode.Format(float64) string`
- Produces:
  - `const DefaultDuration = 10.0; MinDuration = 1.0; MaxDuration = 120.0; MaxLeadIn = 30.0`
  - `type Clip struct { ID string \`json:"id"\`; Start float64 \`json:"start"\`; Duration float64 \`json:"duration"\`; Label string \`json:"label"\`; Approved bool \`json:"approved"\` }`
  - `type Range struct { Start, Duration float64 }`
  - `func Effective(c Clip, leadIn, videoDuration float64) Range`
  - `type Severity string` with `SeverityError = "error"`, `SeverityWarning = "warning"`
  - `type Issue struct { ClipID string \`json:"clipId"\`; Severity Severity \`json:"severity"\`; Message string \`json:"message"\` }`
  - `func Validate(clips []Clip, leadIn, videoDuration float64) []Issue` — never nil, expects clips sorted by Start
  - `func CanProcess(clips []Clip, issues []Issue) bool`
  - `func Reconcile(old, incoming []Clip, leadInChanged bool, newID func() string) []Clip` — returns clips sorted by Start
  - `func Filename(index int, c Clip) string`, `func Slug(s string) string`, `func SafeDirName(title string) string`

- [ ] **Step 1: Write the failing tests** — `internal/clip/clip_test.go`

```go
package clip

import (
	"reflect"
	"testing"
)

func TestEffective(t *testing.T) {
	cases := []struct {
		name          string
		c             Clip
		leadIn, video float64
		want          Range
	}{
		{"plain", Clip{Start: 30, Duration: 10}, 0, 600, Range{30, 10}},
		{"lead-in extends backwards, end unchanged", Clip{Start: 30, Duration: 10}, 2, 600, Range{28, 12}},
		{"lead-in clamped at 0", Clip{Start: 1, Duration: 10}, 3, 600, Range{0, 11}},
		{"trimmed at video end", Clip{Start: 595, Duration: 10}, 0, 600, Range{595, 5}},
		{"unknown video duration", Clip{Start: 595, Duration: 10}, 0, 0, Range{595, 10}},
		{"starts after end", Clip{Start: 700, Duration: 10}, 0, 600, Range{700, 0}},
	}
	for _, tc := range cases {
		if got := Effective(tc.c, tc.leadIn, tc.video); got != tc.want {
			t.Errorf("%s: got %+v; want %+v", tc.name, got, tc.want)
		}
	}
}

func severities(issues []Issue, id string) []Severity {
	out := []Severity{}
	for _, is := range issues {
		if is.ClipID == id {
			out = append(out, is.Severity)
		}
	}
	return out
}

func TestValidate(t *testing.T) {
	clips := []Clip{
		{ID: "a", Start: 10, Duration: 10},    // fine
		{ID: "b", Start: 15, Duration: 10},    // overlaps a
		{ID: "c", Start: 15, Duration: 10},    // same start as b
		{ID: "d", Start: 100, Duration: 0.5},  // too short
		{ID: "e", Start: 200, Duration: 121},  // too long
		{ID: "f", Start: 595, Duration: 10},   // trimmed at end
		{ID: "g", Start: 600, Duration: 10},   // starts at end
	}
	issues := Validate(clips, 0, 600)
	want := map[string][]Severity{
		"a": {},
		"b": {SeverityWarning},
		"c": {SeverityWarning},
		"d": {SeverityError},
		"e": {SeverityError},
		"f": {SeverityWarning},
		"g": {SeverityError},
	}
	for id, w := range want {
		if got := severities(issues, id); !reflect.DeepEqual(got, w) {
			t.Errorf("clip %s severities = %v; want %v (all: %+v)", id, got, w, issues)
		}
	}
	if Validate(nil, 0, 600) == nil {
		t.Error("Validate(nil) returned nil; want empty slice")
	}
}

func TestCanProcess(t *testing.T) {
	ok := []Clip{{ID: "a", Start: 1, Duration: 10, Approved: true}}
	if !CanProcess(ok, Validate(ok, 0, 600)) {
		t.Error("approved valid clip should be processable")
	}
	if CanProcess(nil, nil) {
		t.Error("no clips should not be processable")
	}
	unapproved := []Clip{{ID: "a", Start: 1, Duration: 10}}
	if CanProcess(unapproved, Validate(unapproved, 0, 600)) {
		t.Error("unapproved clip should block")
	}
	bad := []Clip{{ID: "a", Start: 700, Duration: 10, Approved: true}}
	if CanProcess(bad, Validate(bad, 0, 600)) {
		t.Error("error issue should block")
	}
	warn := []Clip{{ID: "a", Start: 595, Duration: 10, Approved: true}}
	if !CanProcess(warn, Validate(warn, 0, 600)) {
		t.Error("warnings must not block")
	}
}

func TestReconcile(t *testing.T) {
	old := []Clip{
		{ID: "a", Start: 10, Duration: 10, Label: "x", Approved: true},
		{ID: "b", Start: 50, Duration: 10, Approved: true},
		{ID: "c", Start: 90, Duration: 10, Approved: true},
	}
	incoming := []Clip{
		{ID: "c", Start: 90, Duration: 10, Label: "  renamed  ", Approved: true}, // label only: keeps approval
		{ID: "a", Start: 10, Duration: 12, Label: "x", Approved: true},           // duration changed: reset
		{ID: "", Start: 5, Duration: 10, Approved: true},                         // new: gets ID, unapproved
		{ID: "zzz", Start: 70, Duration: 10, Approved: true},                     // unknown ID: unapproved
	}
	n := 0
	got := Reconcile(old, incoming, false, func() string { n++; return "new1" })
	want := []Clip{
		{ID: "new1", Start: 5, Duration: 10},
		{ID: "a", Start: 10, Duration: 12, Label: "x"},
		{ID: "zzz", Start: 70, Duration: 10},
		{ID: "c", Start: 90, Duration: 10, Label: "renamed", Approved: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	all := Reconcile(old, old, true, func() string { return "?" })
	for _, c := range all {
		if c.Approved {
			t.Errorf("lead-in change must clear approval, clip %s still approved", c.ID)
		}
	}
}
```

`internal/clip/naming_test.go`:

```go
package clip

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFilename(t *testing.T) {
	cases := []struct {
		i    int
		c    Clip
		want string
	}{
		{3, Clip{Start: 754, Label: "Andi crossover!"}, "03_12-34_andi-crossover.mp4"},
		{1, Clip{Start: 5}, "01_0-05.mp4"},
		{12, Clip{Start: 3723, Label: "  "}, "12_1-02-03.mp4"},
	}
	for _, tc := range cases {
		if got := Filename(tc.i, tc.c); got != tc.want {
			t.Errorf("Filename(%d, %+v) = %q; want %q", tc.i, tc.c, got, tc.want)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Andi Crossover":       "andi-crossover",
		"  --and-1!!  ":        "and-1",
		"Böse Dunk":            "böse-dunk",
		"🏀🏀":                   "",
		strings.Repeat("a", 60): strings.Repeat("a", 40),
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSafeDirName(t *testing.T) {
	cases := map[string]string{
		"Fun Game 12/10: Team A vs B": "Fun Game 12-10- Team A vs B",
		"  ...hidden  ":               "hidden",
		"":                            "untitled",
		`a*b?c"d<e>f|g\h`:             "a-b-c-d-e-f-g-h",
		"🏀 Game Night 🏀":              "🏀 Game Night 🏀",
	}
	for in, want := range cases {
		if got := SafeDirName(in); got != want {
			t.Errorf("SafeDirName(%q) = %q; want %q", in, got, want)
		}
	}
	long := SafeDirName(strings.Repeat("x", 200))
	if utf8.RuneCountInString(long) != 80 {
		t.Errorf("long title length = %d; want 80", utf8.RuneCountInString(long))
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/clip/`
Expected: FAIL (`undefined: Effective` …)

- [ ] **Step 3: Implement** — `internal/clip/clip.go`

```go
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
```

`internal/clip/naming.go`:

```go
package clip

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/krismp/highlighter/internal/timecode"
)

// Filename is "NN_m-ss[_slug].mp4"; index is 1-based play order.
func Filename(index int, c Clip) string {
	name := fmt.Sprintf("%02d_%s", index, strings.ReplaceAll(timecode.Format(c.Start), ":", "-"))
	if s := Slug(c.Label); s != "" {
		name += "_" + s
	}
	return name + ".mp4"
}

// Slug lowercases s, keeps letters and digits and joins everything else with
// single dashes, capped at 40 runes.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := []rune(strings.TrimSuffix(b.String(), "-"))
	if len(out) > 40 {
		out = out[:40]
	}
	return strings.TrimSuffix(string(out), "-")
}

// SafeDirName turns a video title into a folder name safe on macOS.
func SafeDirName(title string) string {
	mapped := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r) {
			return '-'
		}
		return r
	}, title)
	mapped = strings.Trim(strings.TrimSpace(mapped), ".")
	mapped = strings.TrimSpace(mapped)
	if runes := []rune(mapped); len(runes) > 80 {
		mapped = strings.TrimSpace(string(runes[:80]))
	}
	if mapped == "" {
		return "untitled"
	}
	return mapped
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/clip/`
Expected: `ok`. If `TestSafeDirName` fails on `"  ...hidden  "`, the trim order is wrong: TrimSpace, then Trim("."), then TrimSpace again (as written).

- [ ] **Step 5: Commit**

```bash
git add internal/clip
git commit -m "feat(clip): effective ranges, validation, approval reconciliation, naming"
```

---

### Task 3: store package — sessions and settings on disk

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

**Interfaces:**
- Consumes: `clip.Clip`
- Produces:
  - `type Session struct { VideoID \`json:"videoId"\`; URL \`json:"url"\`; Title \`json:"title"\`; Duration float64 \`json:"duration"\`; Width, Height int; LeadIn float64 \`json:"leadIn"\`; Clips []clip.Clip \`json:"clips"\`; UpdatedAt time.Time \`json:"updatedAt"\` }`
  - `type Settings struct { OutputRoot string \`json:"outputRoot"\`; Concurrency int \`json:"concurrency"\` }`
  - `func DefaultSettings(home string) Settings`
  - `var ErrNotFound`
  - `func New(dir string, defaults Settings) (*Store, error)`
  - `(*Store) SaveSession(Session) (Session, error)` (sets UpdatedAt), `LoadSession(id string) (Session, error)`, `RecentSessions(limit int) ([]Session, error)`, `LoadSettings() (Settings, error)`, `SaveSettings(Settings) error`

- [ ] **Step 1: Write the failing tests** — `internal/store/store_test.go`

```go
package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), DefaultSettings("/Users/me"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	s := newStore(t)
	in := Session{VideoID: "jNQXAC9IVRw", URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Title: "Me at the zoo",
		Duration: 19, LeadIn: 1, Clips: []clip.Clip{{ID: "a", Start: 2, Duration: 10}}}
	saved, err := s.SaveSession(in)
	if err != nil || saved.UpdatedAt.IsZero() {
		t.Fatalf("save: %v, %v", saved, err)
	}
	got, err := s.LoadSession("jNQXAC9IVRw")
	if err != nil || got.Title != "Me at the zoo" || len(got.Clips) != 1 || got.LeadIn != 1 {
		t.Fatalf("load: %+v, %v", got, err)
	}
}

func TestLoadMissingAndInvalidIDs(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"aaaaaaaaaaa", "../../etc/x", ""} {
		if _, err := s.LoadSession(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("LoadSession(%q) err = %v; want ErrNotFound", id, err)
		}
	}
	if _, err := s.SaveSession(Session{VideoID: "../evil"}); err == nil {
		t.Error("saving an invalid id must fail")
	}
}

func TestEmptyClipsLoadAsEmptySlice(t *testing.T) {
	s := newStore(t)
	s.SaveSession(Session{VideoID: "jNQXAC9IVRw"})
	got, _ := s.LoadSession("jNQXAC9IVRw")
	if got.Clips == nil {
		t.Error("Clips is nil; want empty slice")
	}
}

func TestRecentSessionsNewestFirst(t *testing.T) {
	s := newStore(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range []string{"aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa3"} {
		at := base.Add(time.Duration(i) * time.Hour)
		s.now = func() time.Time { return at }
		s.SaveSession(Session{VideoID: id})
	}
	got, err := s.RecentSessions(2)
	if err != nil || len(got) != 2 || got[0].VideoID != "aaaaaaaaaa3" || got[1].VideoID != "aaaaaaaaaa2" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestSettingsDefaultsAndSave(t *testing.T) {
	s := newStore(t)
	got, err := s.LoadSettings()
	want := Settings{OutputRoot: filepath.Join("/Users/me", "Movies", "Highlights"), Concurrency: 3}
	if err != nil || got != want {
		t.Fatalf("defaults = %+v, %v; want %+v", got, err, want)
	}
	if err := s.SaveSettings(Settings{OutputRoot: "/tmp/x", Concurrency: 5}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LoadSettings()
	if got.OutputRoot != "/tmp/x" || got.Concurrency != 5 {
		t.Fatalf("after save = %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/store/`
Expected: FAIL (`undefined: New`)

- [ ] **Step 3: Implement** — `internal/store/store.go`

```go
// Package store persists sessions and settings as JSON files.
package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/clip"
)

var ErrNotFound = errors.New("session not found")

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Session is everything known about one video being clipped.
type Session struct {
	VideoID   string      `json:"videoId"`
	URL       string      `json:"url"`
	Title     string      `json:"title"`
	Duration  float64     `json:"duration"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	LeadIn    float64     `json:"leadIn"`
	Clips     []clip.Clip `json:"clips"`
	UpdatedAt time.Time   `json:"updatedAt"`
}

type Settings struct {
	OutputRoot  string `json:"outputRoot"`
	Concurrency int    `json:"concurrency"`
}

func DefaultSettings(home string) Settings {
	return Settings{OutputRoot: filepath.Join(home, "Movies", "Highlights"), Concurrency: 3}
}

type Store struct {
	dir      string
	defaults Settings
	now      func() time.Time
	mu       sync.Mutex
}

func New(dir string, defaults Settings) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, defaults: defaults, now: time.Now}, nil
}

func (s *Store) sessionPath(id string) (string, bool) {
	if !validID.MatchString(id) {
		return "", false
	}
	return filepath.Join(s.dir, "sessions", id+".json"), true
}

func (s *Store) SaveSession(sess Session) (Session, error) {
	path, ok := s.sessionPath(sess.VideoID)
	if !ok {
		return Session{}, fmt.Errorf("invalid video id %q", sess.VideoID)
	}
	sess.UpdatedAt = s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	return sess, writeJSONFile(path, sess)
}

func (s *Store) LoadSession(id string) (Session, error) {
	path, ok := s.sessionPath(id)
	if !ok {
		return Session{}, ErrNotFound
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return Session{}, fmt.Errorf("read session %s: %w", id, err)
	}
	if sess.Clips == nil {
		sess.Clips = []clip.Clip{}
	}
	return sess, nil
}

func (s *Store) RecentSessions(limit int) ([]Session, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "sessions"))
	if err != nil {
		return nil, err
	}
	out := []Session{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		if sess, err := s.LoadSession(id); err == nil {
			out = append(out, sess)
		}
	}
	slices.SortFunc(out, func(a, b Session) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) LoadSettings() (Settings, error) {
	st := s.defaults
	data, err := os.ReadFile(filepath.Join(s.dir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return s.defaults, fmt.Errorf("read settings: %w", err)
	}
	st.OutputRoot = cmp.Or(st.OutputRoot, s.defaults.OutputRoot)
	st.Concurrency = cmp.Or(st.Concurrency, s.defaults.Concurrency)
	return st, nil
}

func (s *Store) SaveSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSONFile(filepath.Join(s.dir, "settings.json"), st)
}

// writeJSONFile writes atomically via a temp file + rename.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/store/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "feat(store): JSON persistence for sessions and settings"
```

---

### Task 4: ytdlp package — URL parsing and stream resolution

**Files:**
- Create: `internal/ytdlp/ytdlp.go`
- Test: `internal/ytdlp/ytdlp_test.go`

**Interfaces:**
- Produces:
  - `var ErrNotYouTube, ErrLive`
  - `func ParseURL(raw string) (id, canonical string, err error)`
  - `const FormatSelector = "bv*[height<=1080]+ba/b[height<=1080]/b"`
  - `func ResolveArgs(url string) []string`
  - `type Stream struct { URL string; Headers map[string]string }`
  - `type Info struct { ID, Title string; Duration float64; Width, Height int; Video Stream; Audio *Stream }` (`Audio == nil` means the video stream is muxed)
  - `func ParseInfo(data []byte) (Info, error)`
  - `type Client struct { Bin string }` with `func (c Client) Resolve(ctx context.Context, url string) (Info, error)`

- [ ] **Step 1: Write the failing tests** — `internal/ytdlp/ytdlp_test.go`

```go
package ytdlp

import (
	"errors"
	"slices"
	"testing"
)

func TestParseURL(t *testing.T) {
	const id = "jNQXAC9IVRw"
	const canonical = "https://www.youtube.com/watch?v=" + id
	ok := []string{
		"https://www.youtube.com/watch?v=" + id,
		"https://www.youtube.com/watch?v=" + id + "&t=754s&list=PL123",
		"http://youtube.com/watch?feature=share&v=" + id,
		"https://m.youtube.com/watch?v=" + id,
		"https://music.youtube.com/watch?v=" + id,
		"https://youtu.be/" + id,
		"https://youtu.be/" + id + "?si=abcDEF",
		"youtu.be/" + id,
		"www.youtube.com/watch?v=" + id,
		"https://www.youtube.com/shorts/" + id,
		"https://www.youtube.com/live/" + id + "?feature=share",
		"https://www.youtube.com/embed/" + id,
		"  https://youtu.be/" + id + "  ",
	}
	for _, in := range ok {
		gotID, gotURL, err := ParseURL(in)
		if err != nil || gotID != id || gotURL != canonical {
			t.Errorf("ParseURL(%q) = %q, %q, %v", in, gotID, gotURL, err)
		}
	}
	bad := []string{"", "hello", "https://vimeo.com/123", "https://www.youtube.com/watch?v=short",
		"https://www.youtube.com/channel/UC123", "ftp://youtu.be/" + id, "https://evil.com/youtu.be/" + id}
	for _, in := range bad {
		if _, _, err := ParseURL(in); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("ParseURL(%q) err = %v; want ErrNotYouTube", in, err)
		}
	}
}

func TestResolveArgs(t *testing.T) {
	got := ResolveArgs("https://www.youtube.com/watch?v=x")
	want := []string{"-J", "--no-playlist", "--no-warnings", "-f", FormatSelector, "-S", "vcodec:h264", "https://www.youtube.com/watch?v=x"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestParseInfoSplitStreams(t *testing.T) {
	data := []byte(`{"id":"abc","title":"Fun Game","duration":3600.5,"width":1920,"height":1080,"live_status":"was_live",
	 "requested_formats":[
	   {"url":"https://v","vcodec":"avc1.640028","acodec":"none","http_headers":{"User-Agent":"UA"}},
	   {"url":"https://a","vcodec":"none","acodec":"mp4a.40.2","http_headers":{"User-Agent":"UA"}}]}`)
	info, err := ParseInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "abc" || info.Title != "Fun Game" || info.Duration != 3600.5 || info.Width != 1920 || info.Height != 1080 {
		t.Errorf("meta = %+v", info)
	}
	if info.Video.URL != "https://v" || info.Video.Headers["User-Agent"] != "UA" || info.Audio == nil || info.Audio.URL != "https://a" {
		t.Errorf("streams = %+v / %+v", info.Video, info.Audio)
	}
}

func TestParseInfoMuxedStream(t *testing.T) {
	info, err := ParseInfo([]byte(`{"id":"abc","title":"t","duration":10,"url":"https://muxed","http_headers":{"A":"b"}}`))
	if err != nil || info.Video.URL != "https://muxed" || info.Audio != nil {
		t.Errorf("got %+v, %v", info, err)
	}
}

func TestParseInfoRejectsLiveAndEmpty(t *testing.T) {
	for _, status := range []string{"is_live", "is_upcoming"} {
		_, err := ParseInfo([]byte(`{"id":"abc","url":"https://x","live_status":"` + status + `"}`))
		if !errors.Is(err, ErrLive) {
			t.Errorf("%s: err = %v; want ErrLive", status, err)
		}
	}
	if _, err := ParseInfo([]byte(`{"id":"abc"}`)); err == nil {
		t.Error("no stream must be an error")
	}
	if _, err := ParseInfo([]byte(`not json`)); err == nil {
		t.Error("bad json must be an error")
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/ytdlp/`
Expected: FAIL (`undefined: ParseURL`)

- [ ] **Step 3: Implement** — `internal/ytdlp/ytdlp.go`

```go
// Package ytdlp resolves YouTube videos to direct stream URLs via yt-dlp.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotYouTube = errors.New("not a YouTube video URL")
	ErrLive       = errors.New("live and upcoming streams are not supported")
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ParseURL extracts the video ID and returns the canonical watch URL.
func ParseURL(raw string) (id, canonical string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", ErrNotYouTube
	}
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "m.youtube.com", "music.youtube.com":
		if u.Path == "/watch" {
			id = u.Query().Get("v")
			break
		}
		for _, prefix := range []string{"/shorts/", "/live/", "/embed/"} {
			if rest, ok := strings.CutPrefix(u.Path, prefix); ok {
				id = strings.Trim(rest, "/")
			}
		}
	}
	if !idPattern.MatchString(id) {
		return "", "", ErrNotYouTube
	}
	return id, "https://www.youtube.com/watch?v=" + id, nil
}

const FormatSelector = "bv*[height<=1080]+ba/b[height<=1080]/b"

func ResolveArgs(url string) []string {
	return []string{"-J", "--no-playlist", "--no-warnings", "-f", FormatSelector, "-S", "vcodec:h264", url}
}

type Stream struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

type Info struct {
	ID       string
	Title    string
	Duration float64
	Width    int
	Height   int
	Video    Stream
	Audio    *Stream // nil when Video is a muxed audio+video stream
}

type rawFormat struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"http_headers"`
	VCodec  string            `json:"vcodec"`
	ACodec  string            `json:"acodec"`
}

type rawInfo struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Duration         float64           `json:"duration"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	LiveStatus       string            `json:"live_status"`
	URL              string            `json:"url"`
	Headers          map[string]string `json:"http_headers"`
	RequestedFormats []rawFormat       `json:"requested_formats"`
}

// ParseInfo reads `yt-dlp -J` output.
func ParseInfo(data []byte) (Info, error) {
	var raw rawInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		return Info{}, fmt.Errorf("parse yt-dlp output: %w", err)
	}
	if raw.LiveStatus == "is_live" || raw.LiveStatus == "is_upcoming" {
		return Info{}, ErrLive
	}
	info := Info{ID: raw.ID, Title: raw.Title, Duration: raw.Duration, Width: raw.Width, Height: raw.Height}
	if len(raw.RequestedFormats) > 0 {
		for _, f := range raw.RequestedFormats {
			s := Stream{URL: f.URL, Headers: f.Headers}
			switch {
			case f.VCodec != "" && f.VCodec != "none":
				info.Video = s
			case f.ACodec != "" && f.ACodec != "none":
				info.Audio = &s
			}
		}
	} else if raw.URL != "" {
		info.Video = Stream{URL: raw.URL, Headers: raw.Headers}
	}
	if info.Video.URL == "" {
		return Info{}, errors.New("yt-dlp returned no playable video stream")
	}
	return info, nil
}

// Client runs the yt-dlp binary.
type Client struct {
	Bin string
}

func (c Client) Resolve(ctx context.Context, url string) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Bin, ResolveArgs(url)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Info{}, fmt.Errorf("yt-dlp: %s", lastLine(stderr.String(), err))
	}
	return ParseInfo(out)
}

func lastLine(s string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	return fallback.Error()
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ytdlp/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/ytdlp
git commit -m "feat(ytdlp): URL parsing and stream resolution"
```

---

### Task 5: ffmpeg package + tool discovery

**Files:**
- Create: `internal/ffmpeg/args.go`, `internal/ffmpeg/run.go`, `internal/tools/tools.go`
- Test: `internal/ffmpeg/args_test.go`, `internal/tools/tools_test.go`

**Interfaces:**
- Consumes: `ytdlp.Stream`
- Produces:
  - `type Encoder string`; `VideoToolbox Encoder = "h264_videotoolbox"`, `X264 Encoder = "libx264"`
  - `type CutSpec struct { Video ytdlp.Stream; Audio *ytdlp.Stream; Start, Duration float64; Encoder Encoder; Output string }`
  - `func CutArgs(s CutSpec) []string`, `func ConcatArgs(listFile, output string) []string`, `func ConcatList(paths []string) string`, `func ProgressFraction(line string, duration float64) (float64, bool)`
  - `type Runner struct { Bin string }` with `Cut(ctx, CutSpec, func(float64)) error` and `Concat(ctx, inputs []string, output string) error`
  - `func DetectEncoder(ctx context.Context, bin string) Encoder`
  - `tools.ToolStatus{Found bool "found"; Path "path"; Version "version"}`, `tools.Status{YtDlp "ytDlp"; FFmpeg "ffmpeg"; Encoder string "encoder"}`, `func Check(ctx) Status`, `func UpdateYtDlp(ctx) (string, error)`, `func FirstLine(string) string`

- [ ] **Step 1: Write the failing tests** — `internal/ffmpeg/args_test.go`

```go
package ffmpeg

import (
	"slices"
	"testing"

	"github.com/krismp/highlighter/internal/ytdlp"
)

func TestCutArgsSplitStreamsVideoToolbox(t *testing.T) {
	got := CutArgs(CutSpec{
		Video:    ytdlp.Stream{URL: "https://v", Headers: map[string]string{"User-Agent": "UA", "Accept": "*/*"}},
		Audio:    &ytdlp.Stream{URL: "https://a"},
		Start:    754.5,
		Duration: 12,
		Encoder:  VideoToolbox,
		Output:   "/out/03.mp4.part",
	})
	want := []string{
		"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats",
		"-reconnect", "1", "-reconnect_delay_max", "5", "-headers", "Accept: */*\r\nUser-Agent: UA\r\n", "-ss", "754.500", "-i", "https://v",
		"-reconnect", "1", "-reconnect_delay_max", "5", "-ss", "754.500", "-i", "https://a",
		"-t", "12.000", "-map", "0:v:0", "-map", "1:a:0",
		"-c:v", "h264_videotoolbox", "-b:v", "8M", "-maxrate", "12M", "-bufsize", "16M",
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k", "-movflags", "+faststart", "-f", "mp4", "/out/03.mp4.part",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCutArgsMuxedStreamX264(t *testing.T) {
	got := CutArgs(CutSpec{Video: ytdlp.Stream{URL: "https://m"}, Start: 0, Duration: 10, Encoder: X264, Output: "o.mp4"})
	for _, frag := range [][]string{
		{"-map", "0:v:0", "-map", "0:a:0?"},
		{"-c:v", "libx264", "-preset", "veryfast", "-crf", "20"},
	} {
		if !containsSeq(got, frag) {
			t.Errorf("args %q missing %q", got, frag)
		}
	}
	if slices.Contains(got, "-scale") || slices.Contains(got, "-vf") {
		t.Error("must not scale or filter video: aspect ratio is preserved by not touching it")
	}
}

func containsSeq(haystack, needle []string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if slices.Equal(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func TestConcat(t *testing.T) {
	list := ConcatList([]string{"/a/01.mp4", "/a/it's.mp4"})
	if list != "file '/a/01.mp4'\nfile '/a/it'\\''s.mp4'\n" {
		t.Errorf("list = %q", list)
	}
	args := ConcatArgs("/a/list.txt", "/a/full.mp4.part")
	if !containsSeq(args, []string{"-f", "concat", "-safe", "0", "-i", "/a/list.txt", "-c", "copy"}) || args[len(args)-1] != "/a/full.mp4.part" {
		t.Errorf("args = %q", args)
	}
}

func TestProgressFraction(t *testing.T) {
	cases := []struct {
		line string
		dur  float64
		want float64
		ok   bool
	}{
		{"out_time_us=5000000", 10, 0.5, true},
		{"out_time_us=15000000", 10, 1, true},
		{"out_time_us=N/A", 10, 0, false},
		{"out_time_us=-1", 10, 0, false},
		{"frame=12", 10, 0, false},
		{"out_time_us=5000000", 0, 0, false},
	}
	for _, tc := range cases {
		got, ok := ProgressFraction(tc.line, tc.dur)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ProgressFraction(%q, %v) = %v, %v", tc.line, tc.dur, got, ok)
		}
	}
}
```

`internal/tools/tools_test.go`:

```go
package tools

import "testing"

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"2026.09.01\n":                          "2026.09.01",
		"ffmpeg version 8.0 Copyright\nbuilt…": "ffmpeg version 8.0 Copyright",
		"":                                      "",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q; want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/ffmpeg/ ./internal/tools/`
Expected: FAIL (`undefined: CutArgs`, `undefined: FirstLine`)

- [ ] **Step 3: Implement** — `internal/ffmpeg/args.go`

```go
// Package ffmpeg builds and runs the ffmpeg commands that cut clips.
package ffmpeg

import (
	"slices"
	"strconv"
	"strings"

	"github.com/krismp/highlighter/internal/ytdlp"
)

type Encoder string

const (
	VideoToolbox Encoder = "h264_videotoolbox"
	X264         Encoder = "libx264"
)

// CutSpec describes one clip: input-seek on the stream URLs, re-encode
// frame-accurately, never scale (source aspect ratio is kept).
type CutSpec struct {
	Video    ytdlp.Stream
	Audio    *ytdlp.Stream
	Start    float64
	Duration float64
	Encoder  Encoder
	Output   string
}

func CutArgs(s CutSpec) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	args = append(args, inputArgs(s.Video, s.Start)...)
	if s.Audio != nil {
		args = append(args, inputArgs(*s.Audio, s.Start)...)
	}
	args = append(args, "-t", seconds(s.Duration), "-map", "0:v:0")
	if s.Audio != nil {
		args = append(args, "-map", "1:a:0")
	} else {
		args = append(args, "-map", "0:a:0?")
	}
	args = append(args, encoderArgs(s.Encoder)...)
	return append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k",
		"-movflags", "+faststart", "-f", "mp4", s.Output)
}

func inputArgs(st ytdlp.Stream, start float64) []string {
	args := []string{"-reconnect", "1", "-reconnect_delay_max", "5"}
	if h := headerBlob(st.Headers); h != "" {
		args = append(args, "-headers", h)
	}
	return append(args, "-ss", seconds(start), "-i", st.URL)
}

func headerBlob(h map[string]string) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + ": " + h[k] + "\r\n")
	}
	return b.String()
}

func encoderArgs(e Encoder) []string {
	if e == VideoToolbox {
		return []string{"-c:v", string(VideoToolbox), "-b:v", "8M", "-maxrate", "12M", "-bufsize", "16M"}
	}
	return []string{"-c:v", string(X264), "-preset", "veryfast", "-crf", "20"}
}

func seconds(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }

func ConcatArgs(listFile, output string) []string {
	return []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", "-movflags", "+faststart", "-f", "mp4", output}
}

// ConcatList renders an ffmpeg concat-demuxer list file.
func ConcatList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString("file '" + strings.ReplaceAll(p, "'", `'\''`) + "'\n")
	}
	return b.String()
}

// ProgressFraction reads an `-progress` line and returns completion in [0,1].
func ProgressFraction(line string, duration float64) (float64, bool) {
	v, ok := strings.CutPrefix(line, "out_time_us=")
	if !ok || duration <= 0 {
		return 0, false
	}
	us, err := strconv.ParseInt(v, 10, 64)
	if err != nil || us < 0 {
		return 0, false
	}
	return min(1, float64(us)/1e6/duration), true
}
```

`internal/ffmpeg/run.go`:

```go
package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runner executes the ffmpeg binary.
type Runner struct {
	Bin string
}

// Cut writes spec.Output via a ".part" file that is renamed on success.
func (r Runner) Cut(ctx context.Context, spec CutSpec, onProgress func(float64)) error {
	final := spec.Output
	spec.Output = final + ".part"
	defer os.Remove(spec.Output)
	err := r.run(ctx, CutArgs(spec), func(line string) {
		if p, ok := ProgressFraction(line, spec.Duration); ok && onProgress != nil {
			onProgress(p)
		}
	})
	if err != nil {
		return err
	}
	return os.Rename(spec.Output, final)
}

func (r Runner) Concat(ctx context.Context, inputs []string, output string) error {
	list := output + ".txt"
	if err := os.WriteFile(list, []byte(ConcatList(inputs)), 0o644); err != nil {
		return err
	}
	defer os.Remove(list)
	part := output + ".part"
	defer os.Remove(part)
	if err := r.run(ctx, ConcatArgs(list, part), nil); err != nil {
		return err
	}
	return os.Rename(part, output)
}

func (r Runner) run(ctx context.Context, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if onLine != nil {
			onLine(sc.Text())
		}
	}
	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, msg)
	}
	return nil
}

// DetectEncoder test-encodes a tiny clip with VideoToolbox and falls back to libx264.
func DetectEncoder(ctx context.Context, bin string) Encoder {
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=128x128:d=0.2",
		"-c:v", string(VideoToolbox), "-f", "null", "-")
	if cmd.Run() == nil {
		return VideoToolbox
	}
	return X264
}
```

`internal/tools/tools.go`:

```go
// Package tools discovers the external binaries highlighter depends on.
package tools

import (
	"context"
	"os/exec"
	"strings"
)

type ToolStatus struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

type Status struct {
	YtDlp   ToolStatus `json:"ytDlp"`
	FFmpeg  ToolStatus `json:"ffmpeg"`
	Encoder string     `json:"encoder"`
}

func Check(ctx context.Context) Status {
	return Status{
		YtDlp:  probe(ctx, "yt-dlp", "--version"),
		FFmpeg: probe(ctx, "ffmpeg", "-version"),
	}
}

func probe(ctx context.Context, name string, versionArgs ...string) ToolStatus {
	path, err := exec.LookPath(name)
	if err != nil {
		return ToolStatus{}
	}
	out, _ := exec.CommandContext(ctx, path, versionArgs...).Output()
	return ToolStatus{Found: true, Path: path, Version: FirstLine(string(out))}
}

func FirstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// UpdateYtDlp upgrades the Homebrew-installed yt-dlp.
func UpdateYtDlp(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "brew", "upgrade", "yt-dlp").CombinedOutput()
	return string(out), err
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ffmpeg/ ./internal/tools/`
Expected: `ok` for both

- [ ] **Step 5: Commit**

```bash
git add internal/ffmpeg internal/tools
git commit -m "feat(ffmpeg): cut/concat command builders, runner, encoder detection; tool discovery"
```

---

### Task 6: job manager — worker pool, retries, concat, subscribers

**Files:**
- Create: `internal/job/job.go`
- Test: `internal/job/job_test.go`

**Interfaces:**
- Consumes: `clip.Range`, `ytdlp.Info`, `ffmpeg.CutSpec`, `ffmpeg.Encoder`
- Produces:
  - `type Resolver interface { Resolve(ctx context.Context, url string) (ytdlp.Info, error) }`
  - `type Cutter interface { Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error; Concat(ctx context.Context, inputs []string, output string) error }`
  - `type Status string`: `Queued`, `Processing`, `Done`, `Failed` (`"queued"`, …)
  - `type ClipState struct { ClipID "clipId"; File "file"; Status "status"; Progress float64 "progress"; Attempt int "attempt"; Error "error" }`
  - `type ConcatState struct { Requested bool "requested"; Status "status"; File "file"; Error "error" }`
  - `type View struct { SessionID "sessionId"; OutputDir "outputDir"; Clips []ClipState "clips"; Concat ConcatState "concat"; Running bool "running" }`
  - `type Task struct { ClipID string; Range clip.Range; File string }`
  - `type Request struct { SessionID, URL, OutputDir string; Tasks []Task; Concat bool; Encoder ffmpeg.Encoder; Concurrency int }`
  - `const ConcatID = "concat"; ConcatFile = "highlights_full.mp4"`
  - `var ErrBusy, ErrNotFound, ErrNotRetryable`
  - `func NewManager(r Resolver, c Cutter) *Manager` with exported field `Backoff []time.Duration`
  - `(*Manager) Start(Request) (View, error)`, `Retry(sessionID, clipID string) (View, error)`, `View(sessionID string) (View, bool)`, `Subscribe(sessionID string) (current *View, updates <-chan View, cancel func())`

- [ ] **Step 1: Write the failing tests** — `internal/job/job_test.go`

```go
package job

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/ytdlp"
)

type fakeResolver struct {
	calls atomic.Int32
	err   error
}

func (f *fakeResolver) Resolve(ctx context.Context, url string) (ytdlp.Info, error) {
	f.calls.Add(1)
	if f.err != nil {
		return ytdlp.Info{}, f.err
	}
	return ytdlp.Info{ID: "abc", Video: ytdlp.Stream{URL: "https://v"}, Audio: &ytdlp.Stream{URL: "https://a"}}, nil
}

type fakeCutter struct {
	mu        sync.Mutex
	failures  map[string]int // output basename -> failures remaining
	cuts      []ffmpeg.CutSpec
	concats   [][]string
	delay     time.Duration
	gate      chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
}

func (f *fakeCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error {
	n := f.active.Add(1)
	defer f.active.Add(-1)
	for m := f.maxActive.Load(); n > m && !f.maxActive.CompareAndSwap(m, n); m = f.maxActive.Load() {
	}
	if f.gate != nil {
		<-f.gate
	}
	time.Sleep(f.delay)
	onProgress(0.5)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cuts = append(f.cuts, spec)
	if name := filepath.Base(spec.Output); f.failures[name] > 0 {
		f.failures[name]--
		return errors.New("boom")
	}
	return nil
}

func (f *fakeCutter) Concat(ctx context.Context, inputs []string, output string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.concats = append(f.concats, append([]string{output}, inputs...))
	return nil
}

func newManager(c *fakeCutter, r *fakeResolver) *Manager {
	m := NewManager(r, c)
	m.Backoff = []time.Duration{0, 0}
	return m
}

func request(n int, concat bool) Request {
	req := Request{SessionID: "abc", URL: "https://www.youtube.com/watch?v=abc", OutputDir: "/out",
		Concat: concat, Encoder: ffmpeg.X264, Concurrency: 3}
	for i := range n {
		req.Tasks = append(req.Tasks, Task{
			ClipID: string(rune('a' + i)),
			Range:  clip.Range{Start: float64(10 * i), Duration: 10},
			File:   string(rune('a'+i)) + ".mp4",
		})
	}
	return req
}

func waitFor(t *testing.T, m *Manager, cond func(View) bool) View {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := m.View("abc"); ok && cond(v) {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	v, _ := m.View("abc")
	t.Fatalf("condition not met; last view %+v", v)
	return v
}

func idle(v View) bool { return !v.Running }

func TestRunsAllClips(t *testing.T) {
	c := &fakeCutter{}
	m := newManager(c, &fakeResolver{})
	if _, err := m.Start(request(2, false)); err != nil {
		t.Fatal(err)
	}
	v := waitFor(t, m, idle)
	for _, s := range v.Clips {
		if s.Status != Done || s.Progress != 1 || s.Attempt != 1 {
			t.Errorf("clip state %+v", s)
		}
	}
	if v.Concat.Status != "" || len(c.concats) != 0 {
		t.Errorf("concat should not run: %+v", v.Concat)
	}
	slices.SortFunc(c.cuts, func(a, b ffmpeg.CutSpec) int { return int(a.Start - b.Start) })
	got := c.cuts[1]
	if got.Start != 10 || got.Duration != 10 || got.Output != "/out/b.mp4" || got.Encoder != ffmpeg.X264 ||
		got.Video.URL != "https://v" || got.Audio.URL != "https://a" {
		t.Errorf("cut spec %+v", got)
	}
}

func TestRetriesTransientFailuresAndReResolves(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"a.mp4": 2}}
	r := &fakeResolver{}
	m := newManager(c, r)
	m.Start(request(1, false))
	v := waitFor(t, m, idle)
	if s := v.Clips[0]; s.Status != Done || s.Attempt != 3 {
		t.Errorf("state %+v; want done on attempt 3", s)
	}
	if r.calls.Load() != 3 {
		t.Errorf("resolver calls = %d; want 3 (initial + one per retry)", r.calls.Load())
	}
}

func TestGivesUpAfterTwoRetriesThenManualRetry(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"b.mp4": 3}}
	m := newManager(c, &fakeResolver{})
	m.Start(request(2, false))
	v := waitFor(t, m, idle)
	if s := v.Clips[1]; s.Status != Failed || s.Error != "boom" || s.Attempt != 3 {
		t.Fatalf("state %+v; want failed after 3 attempts", s)
	}
	if v.Clips[0].Status != Done {
		t.Errorf("other clip must keep going: %+v", v.Clips[0])
	}
	if _, err := m.Retry("abc", "b"); err != nil {
		t.Fatal(err)
	}
	v = waitFor(t, m, func(v View) bool { return v.Clips[1].Status == Done })
	if v.Running {
		t.Error("job should be idle after retry")
	}
}

func TestRetryErrors(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{})
	if _, err := m.Retry("abc", "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no job: err = %v", err)
	}
	m.Start(request(1, false))
	waitFor(t, m, idle)
	if _, err := m.Retry("abc", "a"); !errors.Is(err, ErrNotRetryable) {
		t.Errorf("done clip: err = %v", err)
	}
	if _, err := m.Retry("abc", "zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown clip: err = %v", err)
	}
}

func TestResolveFailureFailsClip(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{err: errors.New("yt-dlp: ERROR: Private video")})
	m.Start(request(1, false))
	v := waitFor(t, m, idle)
	if s := v.Clips[0]; s.Status != Failed || s.Error != "yt-dlp: ERROR: Private video" {
		t.Errorf("state %+v", s)
	}
}

func TestRespectsConcurrency(t *testing.T) {
	c := &fakeCutter{delay: 20 * time.Millisecond}
	m := newManager(c, &fakeResolver{})
	req := request(6, false)
	req.Concurrency = 2
	m.Start(req)
	waitFor(t, m, idle)
	if got := c.maxActive.Load(); got > 2 {
		t.Errorf("max concurrent cuts = %d; want <= 2", got)
	}
}

func TestConcatAfterAllDoneInOrder(t *testing.T) {
	c := &fakeCutter{}
	m := newManager(c, &fakeResolver{})
	m.Start(request(3, true))
	v := waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
	want := []string{"/out/highlights_full.mp4", "/out/a.mp4", "/out/b.mp4", "/out/c.mp4"}
	if len(c.concats) != 1 || !slices.Equal(c.concats[0], want) {
		t.Errorf("concats = %v; want [%v]", c.concats, want)
	}
	if v.Concat.File != ConcatFile || v.Running {
		t.Errorf("concat state %+v running=%v", v.Concat, v.Running)
	}
}

func TestConcatWaitsForManualRetry(t *testing.T) {
	c := &fakeCutter{failures: map[string]int{"b.mp4": 3}}
	m := newManager(c, &fakeResolver{})
	m.Start(request(2, true))
	v := waitFor(t, m, idle)
	if v.Concat.Status != Queued || len(c.concats) != 0 {
		t.Fatalf("concat must wait: %+v", v.Concat)
	}
	m.Retry("abc", "b")
	waitFor(t, m, func(v View) bool { return v.Concat.Status == Done })
}

func TestStartWhileRunningIsBusy(t *testing.T) {
	c := &fakeCutter{gate: make(chan struct{})}
	m := newManager(c, &fakeResolver{})
	m.Start(request(1, false))
	if _, err := m.Start(request(1, false)); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v; want ErrBusy", err)
	}
	close(c.gate)
	waitFor(t, m, idle)
	if _, err := m.Start(request(1, false)); err != nil {
		t.Errorf("restart after idle: %v", err)
	}
}

func TestSubscribeReceivesUpdates(t *testing.T) {
	m := newManager(&fakeCutter{}, &fakeResolver{})
	current, updates, cancel := m.Subscribe("abc")
	defer cancel()
	if current != nil {
		t.Fatalf("no job yet, current = %+v", current)
	}
	m.Start(request(2, false))
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v := <-updates:
			if !v.Running && v.Clips[0].Status == Done && v.Clips[1].Status == Done {
				return
			}
		case <-timeout:
			t.Fatal("never received the final view")
		}
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/job/`
Expected: FAIL (`undefined: NewManager`)

- [ ] **Step 3: Implement** — `internal/job/job.go`

```go
// Package job runs clip-cutting jobs: a worker pool per session, automatic
// retries with re-resolved stream URLs, the optional combined reel, and
// latest-state fan-out to subscribers.
//
// Lock order: Manager.mu → job.mu → Manager.subsMu. Never take a lock to the
// left while holding one to the right.
package job

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/ytdlp"
)

var (
	ErrBusy         = errors.New("a job is already running for this video")
	ErrNotFound     = errors.New("no such job or clip")
	ErrNotRetryable = errors.New("only failed items can be retried")
)

const (
	ConcatID   = "concat"
	ConcatFile = "highlights_full.mp4"
)

type Resolver interface {
	Resolve(ctx context.Context, url string) (ytdlp.Info, error)
}

type Cutter interface {
	Cut(ctx context.Context, spec ffmpeg.CutSpec, onProgress func(float64)) error
	Concat(ctx context.Context, inputs []string, output string) error
}

type Status string

const (
	Queued     Status = "queued"
	Processing Status = "processing"
	Done       Status = "done"
	Failed     Status = "failed"
)

type ClipState struct {
	ClipID   string  `json:"clipId"`
	File     string  `json:"file"`
	Status   Status  `json:"status"`
	Progress float64 `json:"progress"`
	Attempt  int     `json:"attempt"`
	Error    string  `json:"error"`
}

type ConcatState struct {
	Requested bool   `json:"requested"`
	Status    Status `json:"status"`
	File      string `json:"file"`
	Error     string `json:"error"`
}

type View struct {
	SessionID string      `json:"sessionId"`
	OutputDir string      `json:"outputDir"`
	Clips     []ClipState `json:"clips"`
	Concat    ConcatState `json:"concat"`
	Running   bool        `json:"running"`
}

type Task struct {
	ClipID string
	Range  clip.Range
	File   string
}

type Request struct {
	SessionID   string
	URL         string
	OutputDir   string
	Tasks       []Task
	Concat      bool
	Encoder     ffmpeg.Encoder
	Concurrency int
}

type Manager struct {
	// Backoff before each automatic retry; its length is the retry count.
	Backoff []time.Duration

	resolver Resolver
	cutter   Cutter

	mu   sync.Mutex
	jobs map[string]*job

	subsMu sync.Mutex
	subs   map[string]map[chan View]struct{}
}

func NewManager(r Resolver, c Cutter) *Manager {
	return &Manager{
		Backoff:  []time.Duration{2 * time.Second, 5 * time.Second},
		resolver: r,
		cutter:   c,
		jobs:     map[string]*job{},
		subs:     map[string]map[chan View]struct{}{},
	}
}

type job struct {
	req Request
	sem chan struct{}

	mu     sync.Mutex
	clips  []ClipState
	concat ConcatState
	active int // clips queued or processing

	resolveMu sync.Mutex
	info      *ytdlp.Info
}

func (j *job) viewLocked() View {
	return View{
		SessionID: j.req.SessionID,
		OutputDir: j.req.OutputDir,
		Clips:     slices.Clone(j.clips),
		Concat:    j.concat,
		Running:   j.active > 0 || j.concat.Status == Processing,
	}
}

func (m *Manager) Start(req Request) (View, error) {
	req.Concurrency = max(req.Concurrency, 1)
	m.mu.Lock()
	if old, ok := m.jobs[req.SessionID]; ok {
		old.mu.Lock()
		running := old.viewLocked().Running
		old.mu.Unlock()
		if running {
			m.mu.Unlock()
			return View{}, ErrBusy
		}
	}
	j := &job{req: req, sem: make(chan struct{}, req.Concurrency), clips: make([]ClipState, len(req.Tasks))}
	for i, t := range req.Tasks {
		j.clips[i] = ClipState{ClipID: t.ClipID, File: t.File, Status: Queued}
	}
	if req.Concat {
		j.concat = ConcatState{Requested: true, Status: Queued, File: ConcatFile}
	}
	j.active = len(req.Tasks)
	m.jobs[req.SessionID] = j
	m.mu.Unlock()

	v := m.update(j, func() {})
	for i := range req.Tasks {
		go m.runClip(j, i)
	}
	return v, nil
}

func (m *Manager) Retry(sessionID, clipID string) (View, error) {
	m.mu.Lock()
	j, ok := m.jobs[sessionID]
	m.mu.Unlock()
	if !ok {
		return View{}, ErrNotFound
	}
	if clipID == ConcatID {
		retryable := false
		v := m.update(j, func() {
			if j.concat.Status == Failed {
				j.concat.Status, j.concat.Error = Processing, ""
				retryable = true
			}
		})
		if !retryable {
			return v, ErrNotRetryable
		}
		go m.runConcat(j)
		return v, nil
	}
	idx, retryable := -1, false
	v := m.update(j, func() {
		for i, c := range j.clips {
			if c.ClipID != clipID {
				continue
			}
			idx = i
			if c.Status == Failed {
				j.clips[i] = ClipState{ClipID: c.ClipID, File: c.File, Status: Queued}
				j.active++
				retryable = true
			}
		}
	})
	if idx < 0 {
		return v, ErrNotFound
	}
	if !retryable {
		return v, ErrNotRetryable
	}
	go m.runClip(j, idx)
	return v, nil
}

func (m *Manager) View(sessionID string) (View, bool) {
	m.mu.Lock()
	j, ok := m.jobs[sessionID]
	m.mu.Unlock()
	if !ok {
		return View{}, false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.viewLocked(), true
}

// Subscribe registers for every View change of the session's current and
// future jobs. Slow subscribers only ever miss intermediate states.
func (m *Manager) Subscribe(sessionID string) (*View, <-chan View, func()) {
	ch := make(chan View, 1)
	add := func() {
		m.subsMu.Lock()
		if m.subs[sessionID] == nil {
			m.subs[sessionID] = map[chan View]struct{}{}
		}
		m.subs[sessionID][ch] = struct{}{}
		m.subsMu.Unlock()
	}
	m.mu.Lock()
	j := m.jobs[sessionID]
	m.mu.Unlock()
	var current *View
	if j != nil {
		j.mu.Lock()
		add()
		v := j.viewLocked()
		current = &v
		j.mu.Unlock()
	} else {
		add()
	}
	cancel := func() {
		m.subsMu.Lock()
		delete(m.subs[sessionID], ch)
		m.subsMu.Unlock()
	}
	return current, ch, cancel
}

// update mutates job state under its lock and broadcasts the new view.
func (m *Manager) update(j *job, fn func()) View {
	j.mu.Lock()
	defer j.mu.Unlock()
	fn()
	v := j.viewLocked()
	m.subsMu.Lock()
	for ch := range m.subs[j.req.SessionID] {
		select { // keep only the latest view in the 1-slot buffer
		case ch <- v:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- v:
			default:
			}
		}
	}
	m.subsMu.Unlock()
	return v
}

func (m *Manager) runClip(j *job, i int) {
	j.sem <- struct{}{}
	defer func() { <-j.sem }()
	var err error
	for attempt := 0; attempt <= len(m.Backoff); attempt++ {
		if attempt > 0 {
			time.Sleep(m.Backoff[attempt-1])
		}
		m.update(j, func() {
			c := &j.clips[i]
			c.Status, c.Attempt, c.Progress, c.Error = Processing, attempt+1, 0, ""
		})
		if err = m.cutOnce(j, i, attempt > 0); err == nil {
			break
		}
	}
	startConcat := false
	m.update(j, func() {
		c := &j.clips[i]
		if err != nil {
			c.Status, c.Error = Failed, err.Error()
		} else {
			c.Status, c.Progress = Done, 1
		}
		j.active--
		if j.concat.Requested && j.concat.Status == Queued && allDone(j.clips) {
			j.concat.Status = Processing
			startConcat = true
		}
	})
	if startConcat {
		m.runConcat(j)
	}
}

func allDone(clips []ClipState) bool {
	for _, c := range clips {
		if c.Status != Done {
			return false
		}
	}
	return true
}

func (m *Manager) cutOnce(j *job, i int, refresh bool) error {
	ctx := context.Background()
	info, err := m.streams(ctx, j, refresh)
	if err != nil {
		return err
	}
	t := j.req.Tasks[i]
	spec := ffmpeg.CutSpec{
		Video: info.Video, Audio: info.Audio,
		Start: t.Range.Start, Duration: t.Range.Duration,
		Encoder: j.req.Encoder,
		Output:  filepath.Join(j.req.OutputDir, t.File),
	}
	return m.cutter.Cut(ctx, spec, func(p float64) {
		m.update(j, func() { j.clips[i].Progress = p })
	})
}

// streams resolves stream URLs once per job, and again on every retry
// because YouTube stream URLs expire and can start returning 403.
func (m *Manager) streams(ctx context.Context, j *job, refresh bool) (ytdlp.Info, error) {
	j.resolveMu.Lock()
	defer j.resolveMu.Unlock()
	if j.info == nil || refresh {
		info, err := m.resolver.Resolve(ctx, j.req.URL)
		if err != nil {
			return ytdlp.Info{}, err
		}
		j.info = &info
	}
	return *j.info, nil
}

func (m *Manager) runConcat(j *job) {
	inputs := make([]string, len(j.req.Tasks))
	for i, t := range j.req.Tasks {
		inputs[i] = filepath.Join(j.req.OutputDir, t.File)
	}
	err := m.cutter.Concat(context.Background(), inputs, filepath.Join(j.req.OutputDir, ConcatFile))
	m.update(j, func() {
		if err != nil {
			j.concat.Status, j.concat.Error = Failed, err.Error()
		} else {
			j.concat.Status = Done
		}
	})
}
```

- [ ] **Step 4: Run the tests (with the race detector) and confirm they pass**

Run: `go test -race -count=3 ./internal/job/`
Expected: `ok`. Any `DATA RACE` report is a failure.

- [ ] **Step 5: Commit**

```bash
git add internal/job
git commit -m "feat(job): worker pool with retries, combined reel and live subscribers"
```

---

### Task 7: HTTP API server

**Files:**
- Create: `internal/server/server.go`, `internal/server/view.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Consumes: everything above. `job.Resolver`, `job.Manager`, `store.Store`, `tools.Status`, `timecode.ParseList`, `clip.*`, `ytdlp.ParseURL`.
- Produces:
  - `type Deps struct { Store *store.Store; Jobs *job.Manager; Resolver job.Resolver; Encoder ffmpeg.Encoder; Health func(context.Context) tools.Status; UpdateYtDlp func(context.Context) (string, error); OpenPath func(string) error; NewID func() string; UI fs.FS }`
  - `func New(d Deps) http.Handler`
  - JSON shapes (the frontend relies on these exact names):
    - `ClipView` = `clip.Clip` fields + `index`, `startText`, `effStart`, `effDuration`, `file`
    - `SessionView` = `videoId, url, title, duration, durationText, leadIn, clips[], issues[], canProcess, outputDir`
    - `RecentSession` = `videoId, title, clipCount, updatedAt`
    - parse response = `{session: SessionView, errors: LineError[]}`

- [ ] **Step 1: Write the failing tests** — `internal/server/server_test.go`

```go
package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ytdlp"
)

const vid = "jNQXAC9IVRw"

type fakeResolver struct{ calls int }

func (f *fakeResolver) Resolve(ctx context.Context, url string) (ytdlp.Info, error) {
	f.calls++
	return ytdlp.Info{ID: vid, Title: "Me at the zoo", Duration: 19, Width: 320, Height: 240,
		Video: ytdlp.Stream{URL: "https://v"}}, nil
}

type fileCutter struct{}

func (fileCutter) Cut(ctx context.Context, spec ffmpeg.CutSpec, p func(float64)) error {
	return os.WriteFile(spec.Output, []byte("clip"), 0o644)
}
func (fileCutter) Concat(ctx context.Context, inputs []string, output string) error {
	return os.WriteFile(output, []byte("reel"), 0o644)
}

type env struct {
	srv      *httptest.Server
	resolver *fakeResolver
	jobs     *job.Manager
	out      string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	st, err := store.New(filepath.Join(dir, "config"), store.Settings{OutputRoot: out, Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	res := &fakeResolver{}
	jobs := job.NewManager(res, fileCutter{})
	jobs.Backoff = []time.Duration{0, 0}
	n := 0
	h := New(Deps{
		Store: st, Jobs: jobs, Resolver: res, Encoder: ffmpeg.X264,
		Health:      func(context.Context) tools.Status { return tools.Status{Encoder: "libx264"} },
		UpdateYtDlp: func(context.Context) (string, error) { return "updated", nil },
		OpenPath:    func(string) error { return nil },
		NewID:       func() string { n++; return fmt.Sprintf("c%d", n) },
		UI: fstest.MapFS{
			"index.html":    {Data: []byte("<html>app</html>")},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{srv: srv, resolver: res, jobs: jobs, out: out}
}

func (e *env) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, r)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

func (e *env) open(t *testing.T) SessionView {
	code, data := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://youtu.be/" + vid + "?si=x"})
	if code != 200 {
		t.Fatalf("open: %d %s", code, data)
	}
	return decode[SessionView](t, data)
}

func (e *env) addClips(t *testing.T, text string) parseResponse {
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/clips/parse", map[string]string{"text": text})
	if code != 200 {
		t.Fatalf("parse: %d %s", code, data)
	}
	return decode[parseResponse](t, data)
}

func (e *env) approveAll(t *testing.T, v SessionView) SessionView {
	clips := make([]map[string]any, len(v.Clips))
	for i, c := range v.Clips {
		clips[i] = map[string]any{"id": c.ID, "start": c.Start, "duration": c.Duration, "label": c.Label, "approved": true}
	}
	code, data := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": v.LeadIn, "clips": clips})
	if code != 200 {
		t.Fatalf("put: %d %s", code, data)
	}
	return decode[SessionView](t, data)
}

func TestOpenVideo(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do(t, "POST", "/api/sessions", map[string]string{"url": "https://vimeo.com/1"}); code != 400 {
		t.Errorf("non-youtube: %d; want 400", code)
	}
	v := e.open(t)
	if v.VideoID != vid || v.Title != "Me at the zoo" || v.DurationText != "0:19" || v.Clips == nil || v.Issues == nil || v.CanProcess {
		t.Errorf("view %+v", v)
	}
	if !strings.HasSuffix(v.OutputDir, filepath.Join("out", "Me at the zoo")) {
		t.Errorf("outputDir %q", v.OutputDir)
	}
	e.open(t)
	if e.resolver.calls != 1 {
		t.Errorf("resolver calls = %d; want 1 (existing session reused)", e.resolver.calls)
	}
	code, data := e.do(t, "GET", "/api/sessions", nil)
	recent := decode[[]RecentSession](t, data)
	if code != 200 || len(recent) != 1 || recent[0].VideoID != vid {
		t.Errorf("recent %d %s", code, data)
	}
}

func TestParseClips(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	r := e.addClips(t, "0:05 Andi crossover\nbanana\n0:02")
	if len(r.Errors) != 1 || r.Errors[0].Line != 2 {
		t.Errorf("errors %+v", r.Errors)
	}
	c := r.Session.Clips
	if len(c) != 2 || c[0].Start != 2 || c[1].Start != 5 || c[1].Label != "Andi crossover" || c[0].Duration != 10 {
		t.Fatalf("clips %+v", c)
	}
	if c[0].Index != 1 || c[1].File != "02_0-05_andi-crossover.mp4" || c[1].StartText != "0:05" || c[1].EffDuration != 10 {
		t.Errorf("clip view %+v", c[1])
	}
	// 2–12 overlaps 5–15 → exactly one overlap warning on the second clip; nothing past 0:19
	if len(r.Session.Issues) != 1 || r.Session.Issues[0].ClipID != c[1].ID || r.Session.Issues[0].Severity != "warning" {
		t.Errorf("issues %+v", r.Session.Issues)
	}
}

func TestApprovalResetsOnTimingChange(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01\n0:08").Session)
	if !v.CanProcess {
		t.Fatalf("all approved should be processable: %+v", v)
	}
	clips := []map[string]any{
		{"id": v.Clips[0].ID, "start": 1, "duration": 6, "label": "", "approved": true},
		{"id": v.Clips[1].ID, "start": 8, "duration": 10, "label": "renamed", "approved": true},
	}
	_, data := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": 0, "clips": clips})
	v = decode[SessionView](t, data)
	if v.Clips[0].Approved || !v.Clips[1].Approved || v.CanProcess {
		t.Errorf("after edit %+v", v.Clips)
	}
	if code, _ := e.do(t, "PUT", "/api/sessions/"+vid, map[string]any{"leadIn": 31, "clips": clips}); code != 400 {
		t.Errorf("lead-in 31: %d; want 400", code)
	}
}

func TestProcessRequiresApproval(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	e.addClips(t, "0:01")
	if code, _ := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]bool{"concat": false}); code != 409 {
		t.Errorf("unapproved process: %d; want 409", code)
	}
}

func TestProcessWritesAndServesFiles(t *testing.T) {
	e := newEnv(t)
	e.open(t)
	v := e.approveAll(t, e.addClips(t, "0:01 tip off\n0:08").Session)
	code, data := e.do(t, "POST", "/api/sessions/"+vid+"/process", map[string]bool{"concat": true})
	if code != 202 {
		t.Fatalf("process: %d %s", code, data)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if jv, _ := e.jobs.View(vid); jv.Concat.Status == job.Done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	code, body := e.do(t, "GET", "/api/sessions/"+vid+"/files/"+v.Clips[0].File, nil)
	if code != 200 || string(body) != "clip" {
		t.Errorf("file: %d %q", code, body)
	}
	if code, body = e.do(t, "GET", "/api/sessions/"+vid+"/files/highlights_full.mp4", nil); code != 200 || string(body) != "reel" {
		t.Errorf("reel: %d %q", code, body)
	}
	for _, bad := range []string{"..%2F..%2Fsecret.mp4", "notes.txt", ".hidden.mp4"} {
		if code, _ := e.do(t, "GET", "/api/sessions/"+vid+"/files/"+bad, nil); code == 200 {
			t.Errorf("file %q served; want rejection", bad)
		}
	}
	// SSE sends the current snapshot first
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/sessions/"+vid+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			jv := decode[job.View](t, []byte(line))
			if jv.Running || jv.Clips[0].Status != job.Done {
				t.Errorf("snapshot %+v", jv)
			}
			return
		}
	}
	t.Fatal("no SSE data line")
}

func TestSettings(t *testing.T) {
	e := newEnv(t)
	if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "/x", "concurrency": 0}); code != 400 {
		t.Errorf("concurrency 0: %d", code)
	}
	if code, _ := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "relative/x", "concurrency": 2}); code != 400 {
		t.Errorf("relative root: %d", code)
	}
	code, data := e.do(t, "PUT", "/api/settings", map[string]any{"outputRoot": "~/Clips", "concurrency": 4})
	home, _ := os.UserHomeDir()
	got := decode[store.Settings](t, data)
	if code != 200 || got.OutputRoot != filepath.Join(home, "Clips") || got.Concurrency != 4 {
		t.Errorf("settings %d %+v", code, got)
	}
}

func TestSPAAndUnknownAPI(t *testing.T) {
	e := newEnv(t)
	if code, body := e.do(t, "GET", "/s/"+vid, nil); code != 200 || string(body) != "<html>app</html>" {
		t.Errorf("spa fallback: %d %q", code, body)
	}
	if code, body := e.do(t, "GET", "/assets/app.js", nil); code != 200 || string(body) != "console.log(1)" {
		t.Errorf("asset: %d %q", code, body)
	}
	if code, _ := e.do(t, "GET", "/api/nope", nil); code != 404 {
		t.Errorf("unknown api: %d; want 404", code)
	}
	if code, _ := e.do(t, "GET", "/api/sessions/aaaaaaaaaaa", nil); code != 404 {
		t.Errorf("missing session: %d; want 404", code)
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run: `go test ./internal/server/`
Expected: FAIL (`undefined: New`)

- [ ] **Step 3: Implement the view** — `internal/server/view.go`

```go
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
```

- [ ] **Step 4: Implement the handlers** — `internal/server/server.go`

```go
// Package server exposes the HTTP API and serves the embedded UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/timecode"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ytdlp"
)

type Deps struct {
	Store       *store.Store
	Jobs        *job.Manager
	Resolver    job.Resolver
	Encoder     ffmpeg.Encoder
	Health      func(context.Context) tools.Status
	UpdateYtDlp func(context.Context) (string, error)
	OpenPath    func(string) error
	NewID       func() string
	UI          fs.FS
}

type server struct{ Deps }

func New(d Deps) http.Handler {
	s := &server{d}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/tools/update-ytdlp", s.updateYtDlp)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.openVideo)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("PUT /api/sessions/{id}", s.putSession)
	mux.HandleFunc("POST /api/sessions/{id}/clips/parse", s.parseClips)
	mux.HandleFunc("POST /api/sessions/{id}/process", s.process)
	mux.HandleFunc("POST /api/sessions/{id}/retry/{clipId}", s.retry)
	mux.HandleFunc("GET /api/sessions/{id}/events", s.events)
	mux.HandleFunc("POST /api/sessions/{id}/open-folder", s.openFolder)
	mux.HandleFunc("GET /api/sessions/{id}/files/{name}", s.file)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "not found") })
	mux.Handle("/", spa(d.UI))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, 400, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *server) settings(w http.ResponseWriter) (store.Settings, bool) {
	st, err := s.Store.LoadSettings()
	if err != nil {
		writeError(w, 500, err.Error())
		return st, false
	}
	return st, true
}

func (s *server) loadSession(w http.ResponseWriter, r *http.Request) (store.Session, bool) {
	sess, err := s.Store.LoadSession(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "session not found")
		return sess, false
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return sess, false
	}
	return sess, true
}

// saveAndRespond persists sess and writes its view.
func (s *server) saveAndRespond(w http.ResponseWriter, sess store.Session, wrap func(SessionView) any) {
	saved, err := s.Store.SaveSession(sess)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	st, ok := s.settings(w)
	if !ok {
		return
	}
	v := buildView(saved, st.OutputRoot)
	if wrap != nil {
		writeJSON(w, 200, wrap(v))
		return
	}
	writeJSON(w, 200, v)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Health(r.Context()))
}

func (s *server) updateYtDlp(w http.ResponseWriter, r *http.Request) {
	out, err := s.UpdateYtDlp(r.Context())
	if err != nil {
		writeError(w, 500, strings.TrimSpace(out+"\n"+err.Error()))
		return
	}
	writeJSON(w, 200, map[string]string{"output": out})
}

func (s *server) getSettings(w http.ResponseWriter, r *http.Request) {
	if st, ok := s.settings(w); ok {
		writeJSON(w, 200, st)
	}
}

func (s *server) putSettings(w http.ResponseWriter, r *http.Request) {
	var st store.Settings
	if !readJSON(w, r, &st) {
		return
	}
	if rest, ok := strings.CutPrefix(st.OutputRoot, "~/"); ok {
		home, _ := os.UserHomeDir()
		st.OutputRoot = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(st.OutputRoot) {
		writeError(w, 400, "output folder must be an absolute path")
		return
	}
	if st.Concurrency < 1 || st.Concurrency > 8 {
		writeError(w, 400, "concurrency must be between 1 and 8")
		return
	}
	if err := s.Store.SaveSettings(st); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.Store.RecentSessions(20)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	out := make([]RecentSession, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, RecentSession{VideoID: sess.VideoID, Title: sess.Title, ClipCount: len(sess.Clips), UpdatedAt: sess.UpdatedAt})
	}
	writeJSON(w, 200, out)
}

func (s *server) openVideo(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id, canonical, err := ytdlp.ParseURL(body.URL)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if sess, err := s.Store.LoadSession(id); err == nil {
		if st, ok := s.settings(w); ok {
			writeJSON(w, 200, buildView(sess, st.OutputRoot))
		}
		return
	}
	info, err := s.Resolver.Resolve(r.Context(), canonical)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	s.saveAndRespond(w, store.Session{
		VideoID: id, URL: canonical, Title: info.Title, Duration: info.Duration,
		Width: info.Width, Height: info.Height, Clips: []clip.Clip{},
	}, nil)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	if st, ok := s.settings(w); ok {
		writeJSON(w, 200, buildView(sess, st.OutputRoot))
	}
}

func (s *server) putSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		LeadIn float64     `json:"leadIn"`
		Clips  []clip.Clip `json:"clips"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.LeadIn < 0 || body.LeadIn > clip.MaxLeadIn {
		writeError(w, 400, fmt.Sprintf("lead-in must be between 0 and %gs", clip.MaxLeadIn))
		return
	}
	sess.Clips = clip.Reconcile(sess.Clips, body.Clips, body.LeadIn != sess.LeadIn, s.NewID)
	sess.LeadIn = body.LeadIn
	s.saveAndRespond(w, sess, nil)
}

func (s *server) parseClips(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	entries, lineErrs := timecode.ParseList(body.Text)
	incoming := slices.Clone(sess.Clips)
	for _, e := range entries {
		incoming = append(incoming, clip.Clip{ID: s.NewID(), Start: e.Start, Duration: clip.DefaultDuration, Label: e.Label})
	}
	sess.Clips = clip.Reconcile(sess.Clips, incoming, false, s.NewID)
	s.saveAndRespond(w, sess, func(v SessionView) any { return parseResponse{Session: v, Errors: lineErrs} })
}

func (s *server) process(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.loadSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Concat bool `json:"concat"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	st, ok := s.settings(w)
	if !ok {
		return
	}
	v := buildView(sess, st.OutputRoot)
	if !v.CanProcess {
		writeError(w, 409, "approve every clip and fix errors before processing")
		return
	}
	if err := os.MkdirAll(v.OutputDir, 0o755); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	req := job.Request{SessionID: sess.VideoID, URL: sess.URL, OutputDir: v.OutputDir,
		Concat: body.Concat, Encoder: s.Encoder, Concurrency: st.Concurrency}
	for _, c := range v.Clips {
		req.Tasks = append(req.Tasks, job.Task{ClipID: c.ID, Range: clip.Range{Start: c.EffStart, Duration: c.EffDuration}, File: c.File})
	}
	jv, err := s.Jobs.Start(req)
	if errors.Is(err, job.ErrBusy) {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, jv)
}

func (s *server) retry(w http.ResponseWriter, r *http.Request) {
	jv, err := s.Jobs.Retry(r.PathValue("id"), r.PathValue("clipId"))
	switch {
	case errors.Is(err, job.ErrNotFound):
		writeError(w, 404, err.Error())
	case errors.Is(err, job.ErrNotRetryable):
		writeError(w, 409, err.Error())
	default:
		writeJSON(w, 200, jv)
	}
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming unsupported")
		return
	}
	current, updates, cancel := s.Jobs.Subscribe(r.PathValue("id"))
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	send := func(v job.View) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	if current != nil {
		send(*current)
	} else {
		fmt.Fprint(w, ": no job yet\n\n")
		flusher.Flush()
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-updates:
			send(v)
		}
	}
}

// sessionDir is the running job's folder, or where a job would write.
func (s *server) sessionDir(w http.ResponseWriter, r *http.Request) (string, bool) {
	if jv, ok := s.Jobs.View(r.PathValue("id")); ok {
		return jv.OutputDir, true
	}
	sess, ok := s.loadSession(w, r)
	if !ok {
		return "", false
	}
	st, ok := s.settings(w)
	return outputDir(st.OutputRoot, sess.Title), ok
}

func (s *server) openFolder(w http.ResponseWriter, r *http.Request) {
	dir, ok := s.sessionDir(w, r)
	if !ok {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if err := s.OpenPath(dir); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *server) file(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != filepath.Base(name) || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".mp4") {
		writeError(w, 400, "invalid file name")
		return
	}
	dir, ok := s.sessionDir(w, r)
	if !ok {
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, name))
}

// spa serves built assets and falls back to index.html for client routes.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if _, err := fs.Stat(ui, p); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			http.Error(w, "UI not built: run `make build`", 503)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test -race ./internal/server/`
Expected: `ok`. If the `..%2F..%2Fsecret.mp4` case returns 200, the file handler's `filepath.Base` check is not being reached. Inspect `r.PathValue("name")`.

- [ ] **Step 6: Run the whole suite**

Run: `go test -race ./...`
Expected: every package `ok`

- [ ] **Step 7: Commit**

```bash
git add internal/server
git commit -m "feat(server): JSON API, SSE progress, file serving and SPA fallback"
```

---

### Task 8: binary, embed, Makefile, e2e test

**Files:**
- Create: `internal/ui/ui.go`, `internal/ui/dist/.gitkeep`, `cmd/highlighter/main.go`, `Makefile`, `e2e/e2e_test.go`, `README.md`

**Interfaces:**
- Consumes: `server.New`, `server.Deps`, `job.NewManager`, `ytdlp.Client`, `ffmpeg.Runner`, `ffmpeg.DetectEncoder`, `tools.Check`, `tools.UpdateYtDlp`, `store.New`, `store.DefaultSettings`
- Produces: `ui.FS() fs.FS`. Make targets `build`, `web`, `test`, `e2e`, `dev-api`, `dev-web`.

- [ ] **Step 1: Install the external tools**

```bash
brew install yt-dlp ffmpeg
yt-dlp --version && ffmpeg -hide_banner -encoders | grep h264_videotoolbox
```

Expected: a yt-dlp version line and a `V....D h264_videotoolbox` line.

- [ ] **Step 2: Write the embed package** — `internal/ui/ui.go`

```go
// Package ui embeds the built web app (web/ → internal/ui/dist).
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
```

```bash
mkdir -p internal/ui/dist && touch internal/ui/dist/.gitkeep
```

- [ ] **Step 3: Write main** — `cmd/highlighter/main.go`

```go
// Command highlighter serves the clipper UI on localhost.
package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/server"
	"github.com/krismp/highlighter/internal/store"
	"github.com/krismp/highlighter/internal/tools"
	"github.com/krismp/highlighter/internal/ui"
	"github.com/krismp/highlighter/internal/ytdlp"
)

func main() {
	port := flag.Int("port", 7777, "port to listen on (127.0.0.1 only)")
	noOpen := flag.Bool("no-open", false, "do not open the browser on start")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.New(filepath.Join(home, ".config", "highlighter"), store.DefaultSettings(home))
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	status := tools.Check(ctx)
	if !status.YtDlp.Found || !status.FFmpeg.Found {
		log.Println("warning: yt-dlp and ffmpeg are required — run: brew install yt-dlp ffmpeg")
	}
	encoder := ffmpeg.X264
	if status.FFmpeg.Found {
		encoder = ffmpeg.DetectEncoder(ctx, status.FFmpeg.Path)
	}
	log.Printf("video encoder: %s", encoder)

	resolver := ytdlp.Client{Bin: cmp.Or(status.YtDlp.Path, "yt-dlp")}
	cutter := ffmpeg.Runner{Bin: cmp.Or(status.FFmpeg.Path, "ffmpeg")}
	handler := server.New(server.Deps{
		Store:    st,
		Jobs:     job.NewManager(resolver, cutter),
		Resolver: resolver,
		Encoder:  encoder,
		Health: func(ctx context.Context) tools.Status {
			s := tools.Check(ctx)
			s.Encoder = string(encoder)
			return s
		},
		UpdateYtDlp: tools.UpdateYtDlp,
		OpenPath:    func(p string) error { return exec.Command("open", p).Run() },
		NewID:       newID,
		UI:          ui.FS(),
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	url := "http://" + addr
	log.Printf("highlighter running at %s", url)
	if !*noOpen {
		_ = exec.Command("open", url).Start()
	}
	log.Fatal(http.Serve(ln, handler))
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Write the Makefile**

```makefile
.PHONY: build web test e2e dev-api dev-web

build: web
	go build -o bin/highlighter ./cmd/highlighter

web:
	cd web && pnpm install --frozen-lockfile && pnpm build
	touch internal/ui/dist/.gitkeep

test:
	go test -race ./...

e2e:
	go test -tags e2e -count=1 -timeout 5m ./e2e/...

dev-api:
	go run ./cmd/highlighter -no-open

dev-web:
	cd web && pnpm dev
```

(`web` will not work until Task 9 creates `web/`. Until then, check the Go side with `go build ./...`.)

- [ ] **Step 5: Write the e2e test** — `e2e/e2e_test.go`

```go
//go:build e2e

// Package e2e clips a real public video with the real yt-dlp and ffmpeg.
package e2e

import (
	"context"
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/clip"
	"github.com/krismp/highlighter/internal/ffmpeg"
	"github.com/krismp/highlighter/internal/job"
	"github.com/krismp/highlighter/internal/ytdlp"
)

const url = "https://www.youtube.com/watch?v=jNQXAC9IVRw" // "Me at the zoo", 19 s

func TestClipRealVideo(t *testing.T) {
	ytBin, err1 := exec.LookPath("yt-dlp")
	ffBin, err2 := exec.LookPath("ffmpeg")
	if err1 != nil || err2 != nil {
		t.Skip("yt-dlp and ffmpeg must be installed")
	}
	ctx := context.Background()
	resolver := ytdlp.Client{Bin: ytBin}
	info, err := resolver.Resolve(ctx, url)
	if err != nil {
		t.Fatal(err)
	}

	out := t.TempDir()
	m := job.NewManager(resolver, ffmpeg.Runner{Bin: ffBin})
	_, err = m.Start(job.Request{
		SessionID: "jNQXAC9IVRw", URL: url, OutputDir: out, Concurrency: 1,
		Encoder: ffmpeg.DetectEncoder(ctx, ffBin),
		Tasks:   []job.Task{{ClipID: "a", Range: clip.Range{Start: 5, Duration: 3}, File: "01_0-05.mp4"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		v, _ := m.View("jNQXAC9IVRw")
		if !v.Running {
			if v.Clips[0].Status != job.Done {
				t.Fatalf("clip failed: %+v", v.Clips[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(200 * time.Millisecond)
	}

	probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration", "-of", "json",
		filepath.Join(out, "01_0-05.mp4")).Output()
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Streams []struct{ Width, Height int }
		Format  struct{ Duration string }
	}
	json.Unmarshal(probe, &p)
	dur, _ := strconv.ParseFloat(p.Format.Duration, 64)
	if math.Abs(dur-3) > 0.25 {
		t.Errorf("duration = %.3f; want ≈3", dur)
	}
	if len(p.Streams) != 1 || p.Streams[0].Width != info.Width || p.Streams[0].Height != info.Height {
		t.Errorf("dimensions = %+v; want %dx%d (source)", p.Streams, info.Width, info.Height)
	}
}
```

- [ ] **Step 6: Write `README.md`**

````markdown
# highlighter

Paste a YouTube URL, mark the start of each play, review each clip in the
embedded player, and get one MP4 per play. The source aspect ratio is kept.

## Requirements

```bash
brew install go node pnpm yt-dlp ffmpeg
```

## Run

```bash
make build && ./bin/highlighter   # opens http://127.0.0.1:7777
```

Clips are written to `~/Movies/Highlights/<video title>/`, which can be changed in Settings.

## Develop

```bash
make dev-api   # Go API on :7777
make dev-web   # Vite on :5173, proxies /api to :7777
make test      # unit tests
make e2e       # clips a real video (network + yt-dlp + ffmpeg)
```

If downloads start failing, click **Update yt-dlp** on the home page (`brew upgrade yt-dlp`).
````

- [ ] **Step 7: Verify**

Run: `go build ./... && go vet ./... && go test -race ./... && make e2e`
Expected: everything passes. The e2e test prints `ok` after roughly 10–30 s.
Then run `go run ./cmd/highlighter -no-open` and, in another terminal, `curl -s localhost:7777/api/health`.
Expected: JSON with `"found":true` for both tools and `"encoder":"h264_videotoolbox"`. `curl -s localhost:7777/` returns `UI not built: run \`make build\``.

- [ ] **Step 8: Commit**

```bash
git add internal/ui cmd Makefile e2e README.md
git commit -m "feat: highlighter binary, Makefile, e2e test against real tools"
```

---

### Task 9: Web app scaffold — Vite, Tailwind, shadcn, router, API client, Home page

**Files:**
- Create: `web/` (Vite scaffold), `web/vite.config.ts`, `web/src/main.tsx`, `web/src/router.tsx`, `web/src/lib/api.ts`, `web/src/routes/HomePage.tsx`, `web/src/components/ToolsBanner.tsx`, `web/src/components/SettingsCard.tsx`
- Modify: `web/tsconfig.json`, `web/tsconfig.app.json`, `web/src/index.css`, `web/index.html`
- Delete: `web/src/App.tsx`, `web/src/App.css`, `web/src/assets/`

**Interfaces:**
- Consumes: the JSON API from Task 7 (exact field names listed there)
- Produces: `api` object and TS types in `@/lib/api` (used by Tasks 10–11). Routes `/` and `/s/$videoId`. `SessionPage` is a stub that Task 10 replaces.

- [ ] **Step 1: Scaffold**

```bash
pnpm create vite@latest web --template react-ts
cd web
pnpm install
pnpm add @tanstack/react-router @tanstack/react-query tailwindcss @tailwindcss/vite
pnpm add -D @types/node @types/youtube
rm -rf src/App.tsx src/App.css src/assets
```

- [ ] **Step 2: Configure Vite, the path alias and Tailwind**

`web/vite.config.ts`:

```ts
import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(__dirname, "./src") } },
  build: { outDir: "../internal/ui/dist", emptyOutDir: true },
  server: { proxy: { "/api": "http://127.0.0.1:7777" } },
});
```

In both `web/tsconfig.json` and `web/tsconfig.app.json`, add to `compilerOptions`:

```json
"baseUrl": ".",
"paths": { "@/*": ["./src/*"] }
```

and in `web/tsconfig.app.json` add `"types": ["vite/client", "youtube"]` to `compilerOptions`.

Replace `web/src/index.css` with:

```css
@import "tailwindcss";
```

In `web/index.html`, set `<title>highlighter</title>`.

- [ ] **Step 3: Initialise shadcn/ui and add the components**

```bash
pnpm dlx shadcn@latest init -d
pnpm dlx shadcn@latest add button input textarea card badge progress checkbox label
```

Expected: `components.json`, `src/lib/utils.ts` and `src/components/ui/*.tsx` are created, and `src/index.css` gains the theme variables.

- [ ] **Step 4: Write the API client** — `web/src/lib/api.ts`

```ts
export type Severity = "error" | "warning";
export interface Issue { clipId: string; severity: Severity; message: string }
export interface Clip { id: string; start: number; duration: number; label: string; approved: boolean }
export interface ClipView extends Clip { index: number; startText: string; effStart: number; effDuration: number; file: string }
export interface SessionView {
  videoId: string; url: string; title: string; duration: number; durationText: string; leadIn: number;
  clips: ClipView[]; issues: Issue[]; canProcess: boolean; outputDir: string;
}
export type ClipInput = Omit<Clip, "id"> & { id?: string };
export interface RecentSession { videoId: string; title: string; clipCount: number; updatedAt: string }
export interface ToolStatus { found: boolean; path: string; version: string }
export interface Health { ytDlp: ToolStatus; ffmpeg: ToolStatus; encoder: string }
export interface Settings { outputRoot: string; concurrency: number }
export interface LineError { line: number; text: string; err: string }
export type JobStatus = "queued" | "processing" | "done" | "failed";
export interface ClipState { clipId: string; file: string; status: JobStatus; progress: number; attempt: number; error: string }
export interface JobView {
  sessionId: string; outputDir: string; clips: ClipState[]; running: boolean;
  concat: { requested: boolean; status: JobStatus | ""; file: string; error: string };
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText);
  return data as T;
}

export const toInputs = (clips: ClipView[]): ClipInput[] =>
  clips.map(({ id, start, duration, label, approved }) => ({ id, start, duration, label, approved }));

export const api = {
  health: () => request<Health>("GET", "/health"),
  updateYtDlp: () => request<{ output: string }>("POST", "/tools/update-ytdlp"),
  settings: () => request<Settings>("GET", "/settings"),
  saveSettings: (s: Settings) => request<Settings>("PUT", "/settings", s),
  recent: () => request<RecentSession[]>("GET", "/sessions"),
  openVideo: (url: string) => request<SessionView>("POST", "/sessions", { url }),
  session: (id: string) => request<SessionView>("GET", `/sessions/${id}`),
  saveSession: (id: string, body: { leadIn: number; clips: ClipInput[] }) =>
    request<SessionView>("PUT", `/sessions/${id}`, body),
  parseClips: (id: string, text: string) =>
    request<{ session: SessionView; errors: LineError[] }>("POST", `/sessions/${id}/clips/parse`, { text }),
  process: (id: string, concat: boolean) => request<JobView>("POST", `/sessions/${id}/process`, { concat }),
  retry: (id: string, clipId: string) => request<JobView>("POST", `/sessions/${id}/retry/${clipId}`),
  openFolder: (id: string) => request<void>("POST", `/sessions/${id}/open-folder`),
  fileUrl: (id: string, file: string) => `/api/sessions/${id}/files/${encodeURIComponent(file)}`,
  eventsUrl: (id: string) => `/api/sessions/${id}/events`,
};
```

- [ ] **Step 5: Write the router and entry point**

`web/src/router.tsx`:

```tsx
import { createRootRoute, createRoute, createRouter, Link, Outlet } from "@tanstack/react-router";
import { HomePage } from "@/routes/HomePage";
import { SessionPage } from "@/routes/SessionPage";

const rootRoute = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <div className="mx-auto max-w-6xl px-4 py-3">
          <Link to="/" className="font-semibold tracking-tight">🏀 highlighter</Link>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  ),
});

const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage });
const sessionRoute = createRoute({ getParentRoute: () => rootRoute, path: "/s/$videoId", component: SessionPage });

export const router = createRouter({ routeTree: rootRoute.addChildren([indexRoute, sessionRoute]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
```

`web/src/main.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { router } from "@/router";
import "./index.css";

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } } });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
```

Stub `web/src/routes/SessionPage.tsx` (Task 10 replaces it):

```tsx
export function SessionPage() {
  return <p>Session page — built in Task 10.</p>;
}
```

- [ ] **Step 6: Write the Home page and its cards**

`web/src/components/ToolsBanner.tsx`:

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

export function ToolsBanner() {
  const qc = useQueryClient();
  const health = useQuery({ queryKey: ["health"], queryFn: api.health });
  const update = useMutation({ mutationFn: api.updateYtDlp, onSettled: () => qc.invalidateQueries({ queryKey: ["health"] }) });
  if (!health.data) return null;
  const { ytDlp, ffmpeg, encoder } = health.data;
  const missing = !ytDlp.found || !ffmpeg.found;
  return (
    <div className={missing ? "rounded-md border border-destructive p-3 text-sm" : "rounded-md border p-3 text-sm text-muted-foreground"}>
      {missing ? (
        <p>
          yt-dlp and ffmpeg are required. Run <code className="font-mono">brew install yt-dlp ffmpeg</code>, then reload.
        </p>
      ) : (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span>yt-dlp {ytDlp.version}</span>
          <span>encoder {encoder}</span>
          <Button size="sm" variant="outline" disabled={update.isPending} onClick={() => update.mutate()}>
            {update.isPending ? "Updating…" : "Update yt-dlp"}
          </Button>
          {update.data && <span>{update.data.output.trim() || "Up to date"}</span>}
          {update.error && <span className="text-destructive">{update.error.message}</span>}
        </div>
      )}
    </div>
  );
}
```

`web/src/components/SettingsCard.tsx`:

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";

export function SettingsCard() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings });
  const [outputRoot, setOutputRoot] = useState("");
  const [concurrency, setConcurrency] = useState("3");
  useEffect(() => {
    if (!settings.data) return;
    setOutputRoot(settings.data.outputRoot);
    setConcurrency(String(settings.data.concurrency));
  }, [settings.data]);
  const save = useMutation({
    mutationFn: () => api.saveSettings({ outputRoot, concurrency: Number(concurrency) }),
    onSuccess: (s) => qc.setQueryData(["settings"], s),
  });
  return (
    <Card>
      <CardHeader><CardTitle>Settings</CardTitle></CardHeader>
      <CardContent>
        <form className="grid gap-3 sm:grid-cols-[1fr_8rem_auto] sm:items-end" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <div className="grid gap-1.5">
            <Label htmlFor="outputRoot">Output folder</Label>
            <Input id="outputRoot" value={outputRoot} onChange={(e) => setOutputRoot(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="concurrency">Parallel clips</Label>
            <Input id="concurrency" type="number" min={1} max={8} value={concurrency} onChange={(e) => setConcurrency(e.target.value)} />
          </div>
          <Button type="submit" disabled={save.isPending}>Save</Button>
        </form>
        {save.error && <p className="mt-2 text-sm text-destructive">{save.error.message}</p>}
        {save.isSuccess && <p className="mt-2 text-sm text-muted-foreground">Saved.</p>}
      </CardContent>
    </Card>
  );
}
```

`web/src/routes/HomePage.tsx`:

```tsx
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { SettingsCard } from "@/components/SettingsCard";
import { ToolsBanner } from "@/components/ToolsBanner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";

export function HomePage() {
  const navigate = useNavigate();
  const [url, setUrl] = useState("");
  const recent = useQuery({ queryKey: ["recent"], queryFn: api.recent });
  const open = useMutation({
    mutationFn: () => api.openVideo(url),
    onSuccess: (s) => navigate({ to: "/s/$videoId", params: { videoId: s.videoId } }),
  });
  return (
    <div className="grid gap-6">
      <ToolsBanner />
      <Card>
        <CardHeader><CardTitle>Clip a match</CardTitle></CardHeader>
        <CardContent>
          <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(e) => { e.preventDefault(); open.mutate(); }}>
            <Input autoFocus placeholder="https://www.youtube.com/watch?v=…" value={url} onChange={(e) => setUrl(e.target.value)} />
            <Button type="submit" disabled={!url.trim() || open.isPending}>{open.isPending ? "Loading…" : "Open"}</Button>
          </form>
          {open.error && <p className="mt-2 text-sm text-destructive">{open.error.message}</p>}
        </CardContent>
      </Card>
      {recent.data && recent.data.length > 0 && (
        <Card>
          <CardHeader><CardTitle>Recent videos</CardTitle></CardHeader>
          <CardContent>
            <ul className="divide-y">
              {recent.data.map((r) => (
                <li key={r.videoId}>
                  <Link to="/s/$videoId" params={{ videoId: r.videoId }} className="flex justify-between gap-4 py-2 hover:underline">
                    <span className="truncate">{r.title}</span>
                    <span className="shrink-0 text-sm text-muted-foreground">
                      {r.clipCount} clips · {new Date(r.updatedAt).toLocaleDateString()}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
      <SettingsCard />
    </div>
  );
}
```

- [ ] **Step 7: Build and check by hand**

Run: `cd web && pnpm build`
Expected: `tsc -b` reports no errors, and Vite writes `../internal/ui/dist/index.html`.
Then from the repo root run `make build && ./bin/highlighter` and check:
- The home page loads at `127.0.0.1:7777` with the tool banner showing the yt-dlp version and `h264_videotoolbox`.
- Pasting `https://youtu.be/jNQXAC9IVRw` and clicking Open navigates to `/s/jNQXAC9IVRw` (stub page).
- Going back, "Recent videos" lists "Me at the zoo".
- Opening `vimeo.com/1` shows `not a YouTube video URL`.
- Saving Settings with concurrency `0` shows the server error. `~/Clips` saves as an absolute path.
- Reloading on `/s/jNQXAC9IVRw` still renders (SPA fallback).

- [ ] **Step 8: Commit**

```bash
git add web internal/ui/dist/.gitkeep
git commit -m "feat(web): Vite + TanStack + shadcn scaffold, API client, home page"
```

---

### Task 10: Session page — player, Mark, paste, lead-in, clip review

**Files:**
- Create: `web/src/hooks/useYouTubePlayer.ts`, `web/src/components/ClipRow.tsx`, `web/src/components/AddClips.tsx`
- Modify: `web/src/routes/SessionPage.tsx` (replace the stub)

**Interfaces:**
- Consumes: `api`, `toInputs`, `SessionView`, `ClipView`, `ClipInput`, `Issue`, `LineError` from `@/lib/api`. shadcn `Button`, `Input`, `Textarea`, `Card`, `Checkbox`, `Label`. `cn` from `@/lib/utils`.
- Produces:
  - `useYouTubePlayer(videoId: string, enabled: boolean): { containerRef: RefObject<HTMLDivElement | null>; ready: boolean; previewing: string | null; preview(id: string, start: number, duration: number): void; stopPreview(): void; currentTime(): number }`
  - `SessionPage` renders `<ProcessPanel session={…} />` from `@/components/ProcessPanel` (Task 11). For this task, create that file as a stub: `export function ProcessPanel(_: { session: SessionView }) { return null; }`

- [ ] **Step 1: Write the player hook** — `web/src/hooks/useYouTubePlayer.ts`

```ts
import { useCallback, useEffect, useRef, useState } from "react";

declare global {
  interface Window {
    onYouTubeIframeAPIReady?: () => void;
  }
}

let apiReady: Promise<void> | null = null;

function loadIframeApi(): Promise<void> {
  if (typeof YT !== "undefined" && YT.Player) return Promise.resolve();
  apiReady ??= new Promise((resolve) => {
    const previous = window.onYouTubeIframeAPIReady;
    window.onYouTubeIframeAPIReady = () => {
      previous?.();
      resolve();
    };
    const script = document.createElement("script");
    script.src = "https://www.youtube.com/iframe_api";
    document.head.appendChild(script);
  });
  return apiReady;
}

/**
 * Embeds the YouTube player; preview() plays [start, start+duration] and auto-pauses.
 * Pass enabled=false until the element bound to containerRef is rendered.
 */
export function useYouTubePlayer(videoId: string, enabled: boolean) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const playerRef = useRef<YT.Player | null>(null);
  const timerRef = useRef<number | null>(null);
  const [ready, setReady] = useState(false);
  const [previewing, setPreviewing] = useState<string | null>(null);

  const clearTimer = () => {
    if (timerRef.current !== null) window.clearInterval(timerRef.current);
    timerRef.current = null;
  };

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    loadIframeApi().then(() => {
      if (cancelled || !containerRef.current) return;
      const mount = document.createElement("div");
      containerRef.current.appendChild(mount);
      playerRef.current = new YT.Player(mount, {
        videoId,
        width: "100%",
        height: "100%",
        playerVars: { rel: 0, playsinline: 1 },
        events: { onReady: () => !cancelled && setReady(true) },
      });
    });
    return () => {
      cancelled = true;
      clearTimer();
      playerRef.current?.destroy();
      playerRef.current = null;
      if (containerRef.current) containerRef.current.innerHTML = "";
      setReady(false);
    };
  }, [videoId, enabled]);

  const stopPreview = useCallback(() => {
    clearTimer();
    playerRef.current?.pauseVideo();
    setPreviewing(null);
  }, []);

  const preview = useCallback((id: string, start: number, duration: number) => {
    const player = playerRef.current;
    if (!player) return;
    clearTimer();
    player.seekTo(start, true);
    player.playVideo();
    setPreviewing(id);
    const end = start + duration;
    timerRef.current = window.setInterval(() => {
      if (player.getCurrentTime() >= end) {
        player.pauseVideo();
        clearTimer();
        setPreviewing(null);
      }
    }, 100);
  }, []);

  const currentTime = useCallback(() => playerRef.current?.getCurrentTime() ?? 0, []);

  return { containerRef, ready, previewing, preview, stopPreview, currentTime };
}
```

- [ ] **Step 2: Write the clip row** — `web/src/components/ClipRow.tsx`

```tsx
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import type { ClipInput, ClipView, Issue } from "@/lib/api";
import { cn } from "@/lib/utils";

const MIN = 1;
const MAX = 120;
const clampDuration = (d: number) => Math.min(MAX, Math.max(MIN, Math.round(d * 10) / 10));

interface Props {
  clip: ClipView;
  issues: Issue[];
  busy: boolean;
  playerReady: boolean;
  previewing: boolean;
  onPreview: () => void;
  onStop: () => void;
  onPatch: (patch: Partial<ClipInput>) => void;
  onRemove: () => void;
}

export function ClipRow({ clip, issues, busy, playerReady, previewing, onPreview, onStop, onPatch, onRemove }: Props) {
  const [label, setLabel] = useState(clip.label);
  const [duration, setDuration] = useState(String(clip.duration));
  useEffect(() => setLabel(clip.label), [clip.label]);
  useEffect(() => setDuration(String(clip.duration)), [clip.duration]);

  const commitDuration = () => {
    const d = Number(duration);
    if (Number.isFinite(d) && d > 0 && clampDuration(d) !== clip.duration) onPatch({ duration: clampDuration(d) });
    else setDuration(String(clip.duration));
  };

  return (
    <Card className={cn("gap-2 p-3", clip.approved && "border-green-600/60")}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="w-8 font-mono text-sm text-muted-foreground">#{clip.index}</span>
        <Button size="sm" variant="ghost" disabled={busy || clip.start < 1} onClick={() => onPatch({ start: Math.max(0, clip.start - 1) })} aria-label="Start one second earlier">−1s</Button>
        <span className="w-16 text-center font-mono">{clip.startText}</span>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => onPatch({ start: clip.start + 1 })} aria-label="Start one second later">+1s</Button>
        <Input
          className="min-w-32 flex-1"
          placeholder="Label (optional)"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onBlur={() => label.trim() !== clip.label && onPatch({ label })}
          onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {[-2, 2, 5].map((d) => (
          <Button key={d} size="sm" variant="outline" disabled={busy} onClick={() => onPatch({ duration: clampDuration(clip.duration + d) })}>
            {d > 0 ? `+${d}s` : `−${-d}s`}
          </Button>
        ))}
        <Input
          className="w-20"
          type="number"
          step="0.5"
          min={MIN}
          max={MAX}
          aria-label="Duration in seconds"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
          onBlur={commitDuration}
          onKeyDown={(e) => e.key === "Enter" && commitDuration()}
        />
        <span className="text-sm text-muted-foreground">s</span>
        {previewing ? (
          <Button size="sm" variant="secondary" onClick={onStop}>■ Stop</Button>
        ) : (
          <Button size="sm" variant="secondary" disabled={!playerReady} onClick={onPreview}>▶ Preview</Button>
        )}
        <label className="ml-auto flex items-center gap-2 text-sm">
          <Checkbox checked={clip.approved} disabled={busy} onCheckedChange={(v) => onPatch({ approved: v === true })} />
          Looks good
        </label>
        <Button size="sm" variant="ghost" disabled={busy} onClick={onRemove}>Remove</Button>
      </div>
      {issues.map((issue, i) => (
        <p key={i} className={cn("text-xs", issue.severity === "error" ? "text-destructive" : "text-amber-600")}>
          {issue.severity === "error" ? "✕ " : "⚠ "}
          {issue.message}
        </p>
      ))}
    </Card>
  );
}
```

- [ ] **Step 3: Write the paste box** — `web/src/components/AddClips.tsx`

```tsx
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { api, type LineError, type SessionView } from "@/lib/api";

export function AddClips({ videoId, onAdded }: { videoId: string; onAdded: (s: SessionView) => void }) {
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<LineError[]>([]);
  const add = useMutation({
    mutationFn: () => api.parseClips(videoId, text),
    onSuccess: (r) => {
      onAdded(r.session);
      setErrors(r.errors);
      setText(r.errors.map((e) => e.text).join("\n")); // keep only the lines that need fixing
    },
  });
  return (
    <div className="grid gap-2">
      <Textarea
        rows={5}
        placeholder={"Paste start times, one per line:\n12:34 Andi crossover\n15:02\n1:02:03 buzzer beater"}
        value={text}
        onChange={(e) => setText(e.target.value)}
        className="font-mono text-sm"
      />
      <Button disabled={!text.trim() || add.isPending} onClick={() => add.mutate()}>Add clips</Button>
      {errors.map((e) => (
        <p key={e.line} className="text-xs text-destructive">Line {e.line}: {e.err}</p>
      ))}
      {add.error && <p className="text-xs text-destructive">{add.error.message}</p>}
    </div>
  );
}
```

- [ ] **Step 4: Write the session page** — `web/src/routes/SessionPage.tsx`

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { AddClips } from "@/components/AddClips";
import { ClipRow } from "@/components/ClipRow";
import { ProcessPanel } from "@/components/ProcessPanel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useYouTubePlayer } from "@/hooks/useYouTubePlayer";
import { api, toInputs, type ClipInput, type SessionView } from "@/lib/api";

export function SessionPage() {
  const { videoId } = useParams({ from: "/s/$videoId" });
  const qc = useQueryClient();
  const session = useQuery({ queryKey: ["session", videoId], queryFn: () => api.session(videoId) });
  const player = useYouTubePlayer(videoId, session.data !== undefined);
  const setSession = (v: SessionView) => qc.setQueryData(["session", videoId], v);
  const save = useMutation({
    mutationFn: (body: { leadIn: number; clips: ClipInput[] }) => api.saveSession(videoId, body),
    onSuccess: setSession,
  });
  const [leadIn, setLeadIn] = useState("0");
  useEffect(() => {
    if (session.data) setLeadIn(String(session.data.leadIn));
  }, [session.data?.leadIn]);

  if (session.error) return <p className="text-destructive">{session.error.message}</p>;
  const s = session.data;
  if (!s) return <p className="text-muted-foreground">Loading…</p>;

  const busy = save.isPending; // one save at a time so rapid clicks can't lose edits
  const clips = toInputs(s.clips);
  const update = (next: ClipInput[], nextLeadIn = s.leadIn) => save.mutate({ leadIn: nextLeadIn, clips: next });
  const patch = (id: string, p: Partial<ClipInput>) => update(clips.map((c) => (c.id === id ? { ...c, ...p } : c)));
  const unapproved = s.clips.filter((c) => !c.approved).length;
  const commitLeadIn = () => {
    const v = Number(leadIn);
    if (Number.isFinite(v) && v >= 0 && v <= 30 && v !== s.leadIn) update(clips, v);
    else setLeadIn(String(s.leadIn));
  };

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="grid content-start gap-4 lg:sticky lg:top-4 lg:self-start">
        <div>
          <h1 className="truncate text-lg font-semibold" title={s.title}>{s.title}</h1>
          <p className="text-sm text-muted-foreground">{s.durationText} · saving to {s.outputDir}</p>
        </div>
        <div ref={player.containerRef} className="aspect-video w-full overflow-hidden rounded-md bg-black [&_iframe]:h-full [&_iframe]:w-full" />
        <Button
          disabled={!player.ready || busy}
          onClick={() => update([...clips, { start: Math.floor(player.currentTime()), duration: 10, label: "", approved: false }])}
        >
          ⚑ Mark play at current time
        </Button>
        <AddClips videoId={videoId} onAdded={setSession} />
        <div className="flex items-center gap-2">
          <Label htmlFor="leadIn">Lead-in (s)</Label>
          <Input id="leadIn" className="w-20" type="number" min={0} max={30} step="0.5" value={leadIn}
            onChange={(e) => setLeadIn(e.target.value)} onBlur={commitLeadIn} onKeyDown={(e) => e.key === "Enter" && commitLeadIn()} />
          <span className="text-xs text-muted-foreground">starts every clip this much earlier; changing it clears approvals</span>
        </div>
        {save.error && <p className="text-sm text-destructive">{save.error.message}</p>}
      </section>

      <section className="grid content-start gap-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Clips ({s.clips.length})</h2>
          <Button size="sm" variant="outline" disabled={busy || unapproved === 0} onClick={() => update(clips.map((c) => ({ ...c, approved: true })))}>
            Approve all remaining ({unapproved})
          </Button>
        </div>
        {s.clips.length === 0 && <p className="text-sm text-muted-foreground">No clips yet. Mark plays while watching, or paste start times.</p>}
        {s.clips.map((c) => (
          <ClipRow
            key={c.id}
            clip={c}
            issues={s.issues.filter((i) => i.clipId === c.id)}
            busy={busy}
            playerReady={player.ready}
            previewing={player.previewing === c.id}
            onPreview={() => player.preview(c.id, c.effStart, c.effDuration)}
            onStop={player.stopPreview}
            onPatch={(p) => patch(c.id, p)}
            onRemove={() => update(clips.filter((x) => x.id !== c.id))}
          />
        ))}
        <ProcessPanel session={s} />
      </section>
    </div>
  );
}
```

Stub `web/src/components/ProcessPanel.tsx`:

```tsx
import type { SessionView } from "@/lib/api";

export function ProcessPanel(_: { session: SessionView }) {
  return null;
}
```

- [ ] **Step 5: Build and check by hand**

Run: `cd web && pnpm build` (no TS errors), then from the root `make build && ./bin/highlighter`. Open a real fun-game video and check:
- The player loads. **Mark** adds a clip at the current second. The list stays sorted by start.
- Paste `12:34 Andi crossover`, `1:23, 2:45`, `banana` → 3 clips added, `banana` stays in the textarea with "Line 3: invalid timestamp".
- ▶ Preview seeks to the start and pauses at start + duration (± ~0.1 s). ■ Stop works.
- +2s / −2s / +5s and the number field change the duration. −2s never goes below 1. Changing the duration un-ticks "Looks good". Editing only the label keeps it ticked.
- Two clips 3 s apart show an "overlaps clip #N" warning. A clip pasted after the video's end shows a red error.
- Setting a lead-in of 2 clears every approval. Preview now starts 2 s earlier.
- "Approve all remaining" ticks everything. Reloading the page keeps the state (persisted).
- At ~375 px width, the layout stacks with no horizontal scrolling.

- [ ] **Step 6: Commit**

```bash
git add web/src
git commit -m "feat(web): session review page with player preview, mark, paste and lead-in"
```

---

### Task 11: Process panel — SSE progress, results, retry, open folder

**Files:**
- Create: `web/src/hooks/useJob.ts`
- Modify: `web/src/components/ProcessPanel.tsx` (replace the stub)

**Interfaces:**
- Consumes: `api.process`, `api.retry`, `api.openFolder`, `api.fileUrl`, `api.eventsUrl`, `JobView`, `SessionView`. shadcn `Button`, `Card`, `Checkbox`, `Progress`, `Badge`.
- Produces: `useJob(videoId: string): JobView | null`. The query key `["job", videoId]` is kept up to date from SSE.

- [ ] **Step 1: Write the SSE hook** — `web/src/hooks/useJob.ts`

```ts
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, type JobView } from "@/lib/api";

/** Live job state for a session; the server pushes a full JobView on every change. */
export function useJob(videoId: string): JobView | null {
  const qc = useQueryClient();
  useEffect(() => {
    const es = new EventSource(api.eventsUrl(videoId));
    es.onmessage = (e) => qc.setQueryData(["job", videoId], JSON.parse(e.data) as JobView);
    return () => es.close();
  }, [videoId, qc]);
  const { data } = useQuery<JobView | null>({
    queryKey: ["job", videoId],
    queryFn: () => null,
    initialData: null,
    staleTime: Infinity,
  });
  return data;
}
```

- [ ] **Step 2: Write the panel** — `web/src/components/ProcessPanel.tsx`

```tsx
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Progress } from "@/components/ui/progress";
import { useJob } from "@/hooks/useJob";
import { api, type JobStatus, type JobView, type SessionView } from "@/lib/api";

const badge: Record<JobStatus, "secondary" | "default" | "destructive" | "outline"> = {
  queued: "outline",
  processing: "secondary",
  done: "default",
  failed: "destructive",
};

export function ProcessPanel({ session }: { session: SessionView }) {
  const id = session.videoId;
  const qc = useQueryClient();
  const job = useJob(id);
  const [concat, setConcat] = useState(false);
  const setJob = (v: JobView) => qc.setQueryData(["job", id], v);
  const process = useMutation({ mutationFn: () => api.process(id, concat), onSuccess: setJob });
  const retry = useMutation({ mutationFn: (clipId: string) => api.retry(id, clipId), onSuccess: setJob });
  const openFolder = useMutation({ mutationFn: () => api.openFolder(id) });

  const unapproved = session.clips.filter((c) => !c.approved).length;
  const errors = session.issues.filter((i) => i.severity === "error").length;
  const reason =
    session.clips.length === 0 ? "Add at least one clip."
    : errors > 0 ? `Fix ${errors} error${errors > 1 ? "s" : ""} first.`
    : unapproved > 0 ? `Review and approve ${unapproved} more clip${unapproved > 1 ? "s" : ""}.`
    : null;
  const running = job?.running ?? false;

  return (
    <Card>
      <CardHeader><CardTitle>Process</CardTitle></CardHeader>
      <CardContent className="grid gap-3">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={concat} onCheckedChange={(v) => setConcat(v === true)} />
          Also combine into highlights_full.mp4
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Button disabled={!session.canProcess || running || process.isPending} onClick={() => process.mutate()}>
            {running ? "Processing…" : `Process ${session.clips.length} clips`}
          </Button>
          <Button variant="outline" onClick={() => openFolder.mutate()}>Open folder</Button>
        </div>
        {reason && <p className="text-sm text-muted-foreground">{reason}</p>}
        {process.error && <p className="text-sm text-destructive">{process.error.message}</p>}

        {job && (
          <ul className="grid gap-3">
            {job.clips.map((c) => (
              <li key={c.clipId} className="grid gap-1.5 rounded-md border p-2">
                <div className="flex items-center gap-2 text-sm">
                  <Badge variant={badge[c.status]}>{c.status}</Badge>
                  <span className="truncate font-mono">{c.file}</span>
                  {c.attempt > 1 && c.status !== "done" && <span className="text-xs text-muted-foreground">attempt {c.attempt}</span>}
                  {c.status === "failed" && (
                    <Button size="sm" variant="outline" className="ml-auto" disabled={retry.isPending} onClick={() => retry.mutate(c.clipId)}>Retry</Button>
                  )}
                  {c.status === "done" && (
                    <a className="ml-auto text-sm underline" href={api.fileUrl(id, c.file)} download>Download</a>
                  )}
                </div>
                {c.status === "processing" && <Progress value={Math.round(c.progress * 100)} />}
                {c.status === "failed" && <p className="text-xs text-destructive">{c.error}</p>}
                {c.status === "done" && (
                  <video className="w-full rounded" src={api.fileUrl(id, c.file)} controls preload="metadata" />
                )}
              </li>
            ))}
            {job.concat.requested && (
              <li className="flex items-center gap-2 rounded-md border p-2 text-sm">
                <Badge variant={job.concat.status ? badge[job.concat.status] : "outline"}>{job.concat.status || "queued"}</Badge>
                <span className="font-mono">{job.concat.file}</span>
                {job.concat.status === "queued" && !running && <span className="text-xs text-muted-foreground">waiting for failed clips</span>}
                {job.concat.status === "failed" && (
                  <Button size="sm" variant="outline" className="ml-auto" onClick={() => retry.mutate("concat")}>Retry</Button>
                )}
                {job.concat.status === "done" && (
                  <a className="ml-auto underline" href={api.fileUrl(id, job.concat.file)} download>Download</a>
                )}
              </li>
            )}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
```

- [ ] **Step 3: Build and check by hand**

Run: `cd web && pnpm build`, then `make build && ./bin/highlighter`. On a session with 3+ approved clips, check:
- Process is disabled with a reason until every clip is approved and nothing is red.
- With "combine" ticked, Process shows each clip go queued → processing (progress bar moves) → done. At most *concurrency* clips are processing at once. Then the reel goes to done.
- Each done clip plays inline. Download saves the MP4. "Open folder" opens Finder at `~/Movies/Highlights/<title>/` with `01_…mp4`, `02_…mp4`, … and `highlights_full.mp4`.
- Clips match the reviewed ranges: the start frame is the same moment Preview showed, the length equals the duration (plus lead-in), and the dimensions match the source (`ffprobe` one file).
- Reloading mid-job reconnects and shows live progress again.
- Failure path: turn Wi-Fi off mid-job → clips show "attempt 2/3", then failed with an error. Turn it back on and press Retry → done. The reel then finishes.
- Pressing Process again while running is impossible (button disabled). The server returns 409 if forced.

- [ ] **Step 4: Run the full check**

Run: `go vet ./... && go test -race ./... && make build`
Expected: all green, `bin/highlighter` built.

- [ ] **Step 5: Commit**

```bash
git add web/src
git commit -m "feat(web): process panel with live SSE progress, retry, downloads and open folder"
```

---

## Self-Review Notes

- **Spec coverage:**
  - URL input + acceptance rules → T4 + T9
  - Timestamps (paste + Mark + labels) → T1 + T7 + T10
  - Duration default/min/max and quick buttons → T2 + T10
  - Lead-in → T2 + T7 + T10
  - Strict review → T2 (`Reconcile`/`CanProcess`) + T7 (409) + T10/T11
  - Section-only fetch + frame-accurate encode + VideoToolbox fallback + 1080p cap + aspect ratio → T4 + T5, verified in T8 e2e
  - Concurrency, retries, re-resolve → T6
  - SSE progress → T6 + T7 + T11
  - Output folder + naming → T2 + T7
  - Combined reel → T5 + T6 + T11
  - Persistence + recent → T3 + T7 + T9
  - Tool check + yt-dlp update → T5 + T8 + T9
  - Single binary + Makefile + localhost only → T8
  - e2e → T8
- **Known simplification:** download and encode are one ffmpeg pass, so the UI shows a single "processing %" instead of separate "downloading" and "encoding" phases. This is recorded in the spec.
- **Type consistency:**
  - Go JSON tags in `job.View`, `server.SessionView`, `server.ClipView`, `tools.Status`, `timecode.LineError` and `store.Settings` match `web/src/lib/api.ts`.
  - `ConcatID = "concat"` matches `retry.mutate("concat")`.
