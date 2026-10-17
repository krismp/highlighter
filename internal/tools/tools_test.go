package tools

import "testing"

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"2026.09.01\n":                         "2026.09.01",
		"ffmpeg version 8.0 Copyright\nbuilt…": "ffmpeg version 8.0 Copyright",
		"":                                     "",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q; want %q", in, got, want)
		}
	}
}
