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
		{ID: "a", Start: 10, Duration: 10},   // fine
		{ID: "b", Start: 15, Duration: 10},   // overlaps a
		{ID: "c", Start: 15, Duration: 10},   // same start as b
		{ID: "d", Start: 100, Duration: 0.5}, // too short
		{ID: "e", Start: 200, Duration: 121}, // too long
		{ID: "f", Start: 595, Duration: 10},  // trimmed at end
		{ID: "g", Start: 600, Duration: 10},  // starts at end
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
