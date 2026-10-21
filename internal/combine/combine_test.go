package combine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/krismp/highlighter/internal/ffmpeg"
)

const probeLandscape = `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'a.mp4':
  Metadata:
    major_brand     : isom
  Duration: 00:01:03.50, start: 0.000000, bitrate: 3212 kb/s
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p(progressive), 1280x720 [SAR 1:1 DAR 16:9], 3130 kb/s, 29.97 fps, 29.97 tbr, 15360 tbn (default)
    Metadata:
      handler_name    : VideoHandler
  Stream #0:1[0x2](und): Audio: aac (LC) (mp4a / 0x6134706D), 48000 Hz, stereo, fltp, 160 kb/s (default)
At least one output file must be specified
`

const probeRotatedPhone = `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'IMG_0001.MOV':
  Duration: 00:00:12.04, start: 0.000000, bitrate: 9000 kb/s
  Stream #0:0[0x1](und): Video: hevc (Main) (hvc1 / 0x31637668), yuv420p(tv, bt709), 1920x1080, 8000 kb/s, 30 fps, 30 tbr, 600 tbn (default)
    Metadata:
      handler_name    : Core Media Video
    Side data:
      Display Matrix: rotation of -90.00 degrees
  Stream #0:1[0x2](und): Audio: aac (LC) (mp4a / 0x6134706D), 44100 Hz, mono, fltp, 96 kb/s (default)
`

const probeCoverArt = `Input #0, matroska,webm, from 'x.mkv':
  Duration: 00:00:05.00, start: 0.000000, bitrate: 500 kb/s
  Stream #0:0: Video: mjpeg (Baseline), yuvj420p(pc, bt470bg/unknown/unknown), 600x600, 90k tbr, 90k tbn (attached pic)
  Stream #0:1: Video: vp9 (Profile 0), yuv420p(tv), 640x360, SAR 1:1 DAR 16:9, 25 fps, 25 tbr, 1k tbn (default)
`

func TestParseProbe(t *testing.T) {
	m, err := ParseProbe(probeLandscape)
	if err != nil {
		t.Fatal(err)
	}
	want := Media{Duration: 63.5, Width: 1280, Height: 720, FPS: 29.97, VideoCodec: "h264", PixFmt: "yuv420p",
		HasAudio: true, AudioCodec: "aac", SampleRate: 48000, Channels: "stereo"}
	if m != want {
		t.Errorf("got %+v\nwant %+v", m, want)
	}

	m, err = ParseProbe(probeRotatedPhone)
	if err != nil {
		t.Fatal(err)
	}
	if m.Width != 1080 || m.Height != 1920 || m.Rotation != 270 || m.VideoCodec != "hevc" || m.Channels != "mono" {
		t.Errorf("rotated phone video: %+v", m)
	}

	m, err = ParseProbe(probeCoverArt)
	if err != nil {
		t.Fatal(err)
	}
	if m.VideoCodec != "vp9" || m.Width != 640 || m.FPS != 25 || m.HasAudio {
		t.Errorf("cover art must be skipped: %+v", m)
	}
}

func TestParseProbeRejects(t *testing.T) {
	audioOnly := "  Duration: 00:03:00.00, start: 0.0\n  Stream #0:0: Audio: mp3, 44100 Hz, stereo, fltp, 320 kb/s\n"
	if _, err := ParseProbe(audioOnly); err != ErrNoVideo {
		t.Errorf("audio only: got %v", err)
	}
	noDuration := "  Duration: N/A, bitrate: N/A\n  Stream #0:0: Video: h264, yuv420p, 640x360, 25 fps\n"
	if _, err := ParseProbe(noDuration); err != ErrNoDuration {
		t.Errorf("no duration: got %v", err)
	}
}

func clipMedia(d float64) Media {
	return Media{Duration: d, Width: 1920, Height: 1080, FPS: 30, VideoCodec: "h264", PixFmt: "yuv420p",
		HasAudio: true, AudioCodec: "aac", SampleRate: 48000, Channels: "stereo"}
}

func TestNewPlan(t *testing.T) {
	same := []Input{{Media: clipMedia(10)}, {Media: clipMedia(12.5)}}
	p := NewPlan(same, SizeAuto, TransitionCut)
	if !p.Copy || p.Width != 1920 || p.Height != 1080 || p.Total != 22.5 || p.FPS != 30 {
		t.Errorf("matching clips: %+v", p)
	}
	if NewPlan(same, SizeAuto, TransitionFade).Copy {
		t.Error("fades need an encode")
	}
	if p := NewPlan(same, SizeVertical, TransitionCut); p.Copy || p.Width != 1080 || p.Height != 1920 {
		t.Errorf("vertical: %+v", p)
	}
	odd := clipMedia(5)
	odd.Width, odd.Height, odd.FPS = 853, 481, 60
	mixed := []Input{{Media: odd}, {Media: clipMedia(3)}}
	if p := NewPlan(mixed, SizeAuto, TransitionCut); p.Copy || p.Width != 852 || p.Height != 480 || p.FPS != 60 {
		t.Errorf("mixed: %+v", p)
	}
	silent := clipMedia(4)
	silent.HasAudio = false
	if NewPlan([]Input{{Media: clipMedia(3)}, {Media: silent}}, SizeAuto, TransitionCut).Copy {
		t.Error("a video without sound needs an encode")
	}
}

func TestFilter(t *testing.T) {
	silent := clipMedia(4)
	silent.HasAudio = false
	in := []Input{{Media: clipMedia(10)}, {Media: silent}}
	f := Filter(in, Plan{Width: 1280, Height: 720, FPS: 29.97}, TransitionFade)
	for _, want := range []string{
		"[0:v:0]setpts=PTS-STARTPTS,scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:",
		"fps=29.97,",
		"fade=t=out:st=9.500:d=0.500[v0]",
		"afade=t=out:st=9.500:d=0.500[a0]",
		"fade=t=in:st=0:d=0.500[v1]",
		"anullsrc=r=48000:cl=stereo,atrim=duration=4.000[a1]",
		"[v0][a0][v1][a1]concat=n=2:v=1:a=1[v][a]",
	} {
		if !strings.Contains(f, want) {
			t.Errorf("filter lacks %q:\n%s", want, f)
		}
	}
	if strings.Contains(Filter(in, Plan{Width: 2, Height: 2, FPS: 30}, TransitionCut), "fade") {
		t.Error("cuts must not fade")
	}
}

func TestUniqueName(t *testing.T) {
	dir := t.TempDir()
	if got := uniqueName(dir, "Best plays", ".mp4"); got != "Best plays.mp4" {
		t.Fatal(got)
	}
	os.WriteFile(filepath.Join(dir, "Best plays.mp4"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "Best plays 2.mp4.part"), nil, 0o644)
	if got := uniqueName(dir, "Best plays", ".mp4"); got != "Best plays 3.mp4" {
		t.Fatal(got)
	}
}

// fakeRunner records commands and succeeds by writing the output file.
type fakeRunner struct {
	block chan struct{}
	args  []string
	paths []string
}

func (f *fakeRunner) Run(ctx context.Context, args []string, onLine func(string)) error {
	f.args = args
	onLine("out_time_us=5000000")
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return os.WriteFile(args[len(args)-1], []byte("video"), 0o644)
}

func (f *fakeRunner) Concat(ctx context.Context, inputs []string, output string) error {
	f.paths = inputs
	return os.WriteFile(output, []byte("joined"), 0o644)
}

func waitFor(t *testing.T, m *Manager, status Status) View {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v := m.View(); v.Status == status {
			return v
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status never became %q: %+v", status, m.View())
	return View{}
}

func TestManagerEncodesAndCopies(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{}
	m := NewManager(f)
	silent := clipMedia(5)
	silent.HasAudio = false
	v, err := m.Start(Request{Inputs: []Input{{Path: "a.mp4", Media: clipMedia(5)}, {Path: "b.mp4", Media: silent}},
		Size: SizeAuto, Transition: TransitionCut, Dir: dir, Name: "Reel"})
	if err != nil || !v.Running || v.Copy || v.File != "Reel.mp4" {
		t.Fatalf("start: %+v %v", v, err)
	}
	v = waitFor(t, m, Done)
	if v.Size != 5 || v.Progress != 1 || v.Duration != 10 {
		t.Errorf("done: %+v", v)
	}
	if got := f.args[len(f.args)-1]; got != filepath.Join(dir, "Reel.mp4.part") {
		t.Errorf("encode must write a partial file first, wrote %s", got)
	}

	v, err = m.Start(Request{Inputs: []Input{{Path: "a.mp4", Media: clipMedia(5)}, {Path: "c.mp4", Media: clipMedia(5)}},
		Size: SizeAuto, Transition: TransitionCut, Dir: dir, Name: "Reel"})
	if err != nil || !v.Copy || v.File != "Reel 2.mp4" {
		t.Fatalf("copy start: %+v %v", v, err)
	}
	waitFor(t, m, Done)
	if strings.Join(f.paths, ",") != "a.mp4,c.mp4" {
		t.Errorf("concat inputs: %v", f.paths)
	}
}

func TestManagerCancelAndBusy(t *testing.T) {
	dir := t.TempDir()
	f := &fakeRunner{block: make(chan struct{})}
	m := NewManager(f)
	req := Request{Inputs: []Input{{Path: "a", Media: clipMedia(10)}, {Path: "b", Media: clipMedia(10)}},
		Size: Size720, Transition: TransitionCut, Dir: dir, Name: "x"}
	if _, err := m.Start(req); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(req); err != ErrBusy {
		t.Fatalf("second start: %v", err)
	}
	if m.Reset() {
		t.Fatal("reset while running")
	}
	m.Cancel()
	waitFor(t, m, Canceled)
	if exists(filepath.Join(dir, "x.mp4.part")) || exists(filepath.Join(dir, "x.mp4")) {
		t.Error("a cancelled combine must leave no file")
	}
	if !m.Reset() || m.View().Status != Idle {
		t.Error("reset after cancel")
	}
	if _, err := m.Start(Request{Inputs: req.Inputs[:1], Dir: dir, Name: "y"}); err == nil {
		t.Error("one video is not a combination")
	}
}

// TestRealFFmpeg combines generated videos of different sizes, frame rates
// and with and without sound, with fades, and joins identical ones by copy.
func TestRealFFmpeg(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	gen := func(name string, args ...string) string {
		p := filepath.Join(dir, name)
		cmd := exec.Command(bin, append(append([]string{"-loglevel", "error", "-y"}, args...), p)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generate %s: %v %s", name, err, out)
		}
		return p
	}
	withSound := []string{"-f", "lavfi", "-i", "testsrc2=s=640x360:r=30:d=2", "-f", "lavfi", "-i", "sine=d=2:sample_rate=48000",
		"-ac", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest"}
	a := gen("a.mp4", withSound...)
	b := gen("b.mp4", withSound...)
	silent := gen("silent vertical.mov", "-f", "lavfi", "-i", "testsrc=s=360x640:r=25:d=1.5", "-c:v", "libx264", "-pix_fmt", "yuv420p")

	tool := Tool{Bin: bin}
	ctx := context.Background()
	var inputs []Input
	for _, p := range []string{a, silent, b} {
		m, err := tool.Probe(ctx, p)
		if err != nil {
			t.Fatalf("probe %s: %v", p, err)
		}
		inputs = append(inputs, Input{Path: p, Media: m})
	}
	if inputs[1].Media.HasAudio || inputs[1].Media.Width != 360 || inputs[1].Media.Height != 640 {
		t.Fatalf("silent vertical probe: %+v", inputs[1].Media)
	}
	if jpg, err := tool.Thumbnail(ctx, a, 0.5); err != nil || len(jpg) < 100 || jpg[0] != 0xFF {
		t.Fatalf("thumbnail: %d bytes, %v", len(jpg), err)
	}

	m := NewManager(ffmpeg.Runner{Bin: bin})
	out := filepath.Join(dir, "out")
	if _, err := m.Start(Request{Inputs: inputs, Size: SizeAuto, Transition: TransitionFade, Encoder: ffmpeg.X264, Dir: out, Name: "mixed"}); err != nil {
		t.Fatal(err)
	}
	v := waitForLong(t, m)
	if v.Status != Done {
		t.Fatalf("encode: %+v", v)
	}
	got, err := tool.Probe(ctx, v.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got.Width != 640 || got.Height != 360 || !got.HasAudio || got.Duration < 5.3 || got.Duration > 5.7 {
		t.Errorf("combined file: %+v", got)
	}

	if _, err := m.Start(Request{Inputs: []Input{inputs[0], inputs[2]}, Size: SizeAuto, Transition: TransitionCut, Dir: out, Name: "copied"}); err != nil {
		t.Fatal(err)
	}
	v = waitForLong(t, m)
	if v.Status != Done || !v.Copy {
		t.Fatalf("copy: %+v", v)
	}
	if got, err := tool.Probe(ctx, v.Path()); err != nil || got.Duration < 3.9 || got.Duration > 4.2 {
		t.Errorf("copied file: %+v %v", got, err)
	}
}

func waitForLong(t *testing.T, m *Manager) View {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if v := m.View(); !v.Running {
			return v
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("combine did not finish in 60s")
	return View{}
}
