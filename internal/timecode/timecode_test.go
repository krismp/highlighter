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
		{Start: 754, Label: "Andi crossover"},
		{Start: 5, Label: "fast break"},
		{Start: 83}, {Start: 165}, {Start: 190},
		{Start: 240, Label: "and-one"},
		{Start: 300},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v\nwant      %+v", entries, want)
	}
	if len(errs) != 1 || errs[0].Line != 4 || errs[0].Text != "banana" || errs[0].Err == "" {
		t.Errorf("errs = %+v; want one error for line 4 'banana'", errs)
	}
}

// The user's real-world format: timestamp with a trailing colon, then the play type.
func TestParseListTrailingColonFormat(t *testing.T) {
	text := "3:47: 3PT\n7:35: DREB\n8:22: 2PT\n10:05: 2PT\n12:39: 3PT\n"
	entries, errs := ParseList(text)
	want := []Entry{{Start: 227, Label: "3PT"}, {Start: 455, Label: "DREB"}, {Start: 502, Label: "2PT"}, {Start: 605, Label: "2PT"}, {Start: 759, Label: "3PT"}}
	if len(errs) != 0 || !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v, errs = %+v; want %+v", entries, errs, want)
	}
}

func TestParseListEmptyReturnsEmptySlices(t *testing.T) {
	entries, errs := ParseList("   \n")
	if entries == nil || errs == nil || len(entries) != 0 || len(errs) != 0 {
		t.Errorf("got %#v, %#v; want empty non-nil slices", entries, errs)
	}
}

// "start - end" ranges pick both ends of the clip; the duration follows from them.
func TestParseListRanges(t *testing.T) {
	text := "2:22 - 3:02\n" +
		"4:00-4:15 fast break\n" +
		"5:10 – 5:40: 3PT\n" +
		"1:00:00 to 1:00:30 | buzzer\n" +
		"6:00 ~ 6:07.5\n" +
		"0:05 - fast break\n" + // a dash before a label is still a label
		"1:23 - 45 points\n" + // a bare number is not a range end
		"3:02 - 2:22\n" // end before start
	entries, errs := ParseList(text)
	want := []Entry{
		{Start: 142, Duration: 40},
		{Start: 240, Duration: 15, Label: "fast break"},
		{Start: 310, Duration: 30, Label: "3PT"},
		{Start: 3600, Duration: 30, Label: "buzzer"},
		{Start: 360, Duration: 7.5},
		{Start: 5, Label: "fast break"},
		{Start: 83, Label: "45 points"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v\nwant      %+v", entries, want)
	}
	if len(errs) != 1 || errs[0].Line != 8 || errs[0].Err == "" {
		t.Errorf("errs = %+v; want one error for line 8", errs)
	}
}

// The user's real-world range format: the label comes first.
func TestParseListLabelFirst(t *testing.T) {
	text := "PLAY + AST: 2:22 - 3:02\n" +
		"PLAY: 3:55 - 4:26\n" +
		"Q2 3PT - 10:45 – 11:15\n" +
		"STEAL: 12:05\n" +
		"Player 5 - 1:10\n" + // "5" has no colon, so it stays in the label
		"Game 7\n" + // a bare trailing number is not a time
		"PLAY: 7:40 - 7:60\n" // 60 seconds is invalid
	entries, errs := ParseList(text)
	want := []Entry{
		{Start: 142, Duration: 40, Label: "PLAY + AST"},
		{Start: 235, Duration: 31, Label: "PLAY"},
		{Start: 645, Duration: 30, Label: "Q2 3PT"},
		{Start: 725, Label: "STEAL"},
		{Start: 70, Label: "Player 5"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v\nwant      %+v", entries, want)
	}
	if len(errs) != 2 || errs[0].Line != 6 || errs[1].Line != 7 {
		t.Errorf("errs = %+v; want errors for lines 6 and 7", errs)
	}
}

// A label may itself start with a number ("3 PT"); a bare number followed by
// an m:ss time or range is part of the label, not a start in seconds.
func TestParseListLabelStartingWithNumber(t *testing.T) {
	text := "3 PT: 10:27 - 11:10\n" +
		"2 PT: 5:34\n" +
		"45 dunk\n" + // bare seconds with a label still work
		"83\n"
	entries, errs := ParseList(text)
	want := []Entry{
		{Start: 627, Duration: 43, Label: "3 PT"},
		{Start: 334, Label: "2 PT"},
		{Start: 45, Label: "dunk"},
		{Start: 83},
	}
	if len(errs) != 0 || !reflect.DeepEqual(entries, want) {
		t.Errorf("entries = %+v, errs = %+v\nwant      %+v", entries, errs, want)
	}
}
