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
	CAFile   string // CA bundle for HTTPS inputs; static ffmpeg builds cannot read the OS trust store
}

func CutArgs(s CutSpec) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	args = append(args, inputArgs(s.Video, s.Start, s.CAFile)...)
	if s.Audio != nil {
		args = append(args, inputArgs(*s.Audio, s.Start, s.CAFile)...)
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

func inputArgs(st ytdlp.Stream, start float64, caFile string) []string {
	args := []string{"-reconnect", "1", "-reconnect_delay_max", "5",
		"-rw_timeout", "30000000"} // 30s I/O stall limit, in microseconds
	if caFile != "" {
		args = append(args, "-ca_file", caFile)
	}
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
