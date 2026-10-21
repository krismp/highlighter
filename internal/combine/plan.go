package combine

import (
	"fmt"
	"math"
	"strings"

	"github.com/krismp/highlighter/internal/ffmpeg"
)

// Size is the combined video's frame: the first video's, or a standard one.
type Size string

const (
	SizeAuto     Size = "auto"
	Size1080     Size = "1080p"
	Size720      Size = "720p"
	SizeVertical Size = "vertical"
	SizeSquare   Size = "square"
)

func (s Size) Valid() bool {
	return s == SizeAuto || s == Size1080 || s == Size720 || s == SizeVertical || s == SizeSquare
}

// Transition is what happens between two videos.
type Transition string

const (
	TransitionCut  Transition = "cut"
	TransitionFade Transition = "fade" // fade through black, sound fades too
)

func (t Transition) Valid() bool { return t == TransitionCut || t == TransitionFade }

// FadeSeconds is the length of each fade out and fade in.
const FadeSeconds = 0.5

// Input is one video to combine, in order.
type Input struct {
	Path  string
	Media Media
}

// Plan is how the inputs become one file.
type Plan struct {
	Copy   bool // join the files as they are, without re-encoding
	Width  int  // frame size of the result
	Height int
	FPS    float64
	Total  float64 // seconds
}

// NewPlan picks the frame size and frame rate (the first video's frame rate), and whether stream copy is
// enough: it is when no fades are wanted, the size follows the first video and
// every file has the same H.264/AAC parameters (clips cut by highlighter do).
func NewPlan(inputs []Input, size Size, tr Transition) Plan {
	var p Plan
	for _, in := range inputs {
		p.Total += in.Media.Duration
	}
	if len(inputs) > 0 {
		p.FPS = inputs[0].Media.FPS // like the size: the first video sets the pace
	}
	if p.FPS <= 0 {
		p.FPS = 30 // unknown: a safe default
	}
	p.FPS = min(max(math.Round(p.FPS*1000)/1000, 15), 60)
	switch size {
	case Size1080:
		p.Width, p.Height = 1920, 1080
	case Size720:
		p.Width, p.Height = 1280, 720
	case SizeVertical:
		p.Width, p.Height = 1080, 1920
	case SizeSquare:
		p.Width, p.Height = 1080, 1080
	default:
		p.Width, p.Height = 1920, 1080
		if len(inputs) > 0 {
			p.Width, p.Height = even(inputs[0].Media.Width), even(inputs[0].Media.Height)
		}
	}
	p.Copy = (size == SizeAuto || size == "") && tr != TransitionFade && copyable(inputs)
	return p
}

func even(n int) int { return max(2, n-n%2) }

func copyable(inputs []Input) bool {
	if len(inputs) == 0 {
		return false
	}
	first := inputs[0].Media
	for _, in := range inputs {
		m := in.Media
		if m.VideoCodec != "h264" || m.PixFmt != "yuv420p" || m.Rotation != 0 ||
			!m.HasAudio || m.AudioCodec != "aac" || m.Width != first.Width || m.Height != first.Height ||
			math.Abs(m.FPS-first.FPS) > 0.01 || m.SampleRate != first.SampleRate || m.Channels != first.Channels {
			return false
		}
	}
	return true
}

// Filter is the filter graph that scales and pads every input to the plan's
// frame (keeping its aspect ratio, black bars around it), evens out frame
// rate and sound (silence for videos without any), fades when asked, and
// concatenates everything into [v] and [a].
func Filter(inputs []Input, p Plan, tr Transition) string {
	var b strings.Builder
	var pads strings.Builder
	for i, in := range inputs {
		d := in.Media.Duration
		fade := ""
		afade := ""
		if tr == TransitionFade {
			f := min(FadeSeconds, d/4)
			if i > 0 {
				fade += fmt.Sprintf(",fade=t=in:st=0:d=%s", seconds(f))
				afade += fmt.Sprintf(",afade=t=in:st=0:d=%s", seconds(f))
			}
			if i < len(inputs)-1 {
				fade += fmt.Sprintf(",fade=t=out:st=%s:d=%s", seconds(d-f), seconds(f))
				afade += fmt.Sprintf(",afade=t=out:st=%s:d=%s", seconds(d-f), seconds(f))
			}
		}
		fmt.Fprintf(&b, "[%d:v:0]setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=decrease,"+
			"pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps=%s,format=yuv420p,trim=duration=%s%s[v%d];\n",
			i, p.Width, p.Height, p.Width, p.Height, fpsText(p.FPS), seconds(d), fade, i)
		if in.Media.HasAudio {
			fmt.Fprintf(&b, "[%d:a:0]asetpts=PTS-STARTPTS,aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo,"+
				"apad,atrim=duration=%s%s[a%d];\n", i, seconds(d), afade, i)
		} else {
			fmt.Fprintf(&b, "anullsrc=r=48000:cl=stereo,atrim=duration=%s[a%d];\n", seconds(d), i)
		}
		fmt.Fprintf(&pads, "[v%d][a%d]", i, i)
	}
	fmt.Fprintf(&b, "%sconcat=n=%d:v=1:a=1[v][a]", pads.String(), len(inputs))
	return b.String()
}

func fpsText(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", f), "0"), ".")
}

// EncodeArgs is the ffmpeg command line for an encoded combine. The filter
// graph is read from filterFile (it can outgrow the Windows command line).
func EncodeArgs(inputs []Input, filterFile string, enc ffmpeg.Encoder, output string) []string {
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error", "-progress", "pipe:1", "-nostats"}
	for _, in := range inputs {
		args = append(args, "-i", in.Path)
	}
	args = append(args, "-/filter_complex", filterFile, "-map", "[v]", "-map", "[a]")
	args = append(args, ffmpeg.EncoderArgs(enc)...)
	return append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k",
		"-movflags", "+faststart", "-f", "mp4", output)
}
