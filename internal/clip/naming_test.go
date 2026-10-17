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
		"Andi Crossover":        "andi-crossover",
		"  --and-1!!  ":         "and-1",
		"Böse Dunk":             "böse-dunk",
		"🏀🏀":                    "",
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

func TestReelName(t *testing.T) {
	cases := map[string]string{
		"Me at the zoo":               "me_at_the_zoo",
		"Fun Game Night: Team A vs B": "fun_game_night_team_a_vs_b",
		"  My Reel.MP4 ":              "my_reel",
		"Böse   Dunk!!":               "böse_dunk",
		"pre-season_game":             "pre-season_game",
		"🏀🏀":                          "highlights",
		"":                            "highlights",
		strings.Repeat("ab ", 40):     strings.TrimSuffix(strings.Repeat("ab_", 20), "_"),
	}
	for in, want := range cases {
		if got := ReelName(in); got != want {
			t.Errorf("ReelName(%q) = %q; want %q", in, got, want)
		}
	}
}
