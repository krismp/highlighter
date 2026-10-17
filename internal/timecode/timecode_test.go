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

// The user's real-world format: timestamp with a trailing colon, then the play type.
func TestParseListTrailingColonFormat(t *testing.T) {
	text := "3:47: 3PT\n7:35: DREB\n8:22: 2PT\n10:05: 2PT\n12:39: 3PT\n"
	entries, errs := ParseList(text)
	want := []Entry{{227, "3PT"}, {455, "DREB"}, {502, "2PT"}, {605, "2PT"}, {759, "3PT"}}
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
