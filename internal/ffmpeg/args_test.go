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
