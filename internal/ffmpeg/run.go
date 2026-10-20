package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/krismp/highlighter/internal/proc"
	"os"
	"strings"
)

// Runner executes the ffmpeg binary.
type Runner struct {
	Bin    string
	CAFile string // passed to every cut; see CutSpec.CAFile
}

// Cut writes spec.Output via a ".part" file that is renamed on success.
func (r Runner) Cut(ctx context.Context, spec CutSpec, onProgress func(float64)) error {
	final := spec.Output
	if spec.CAFile == "" {
		spec.CAFile = r.CAFile
	}
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

// Run runs ffmpeg with args, passing each stdout line (the -progress
// output) to onLine, which may be nil.
func (r Runner) Run(ctx context.Context, args []string, onLine func(string)) error {
	return r.run(ctx, args, onLine)
}

func (r Runner) run(ctx context.Context, args []string, onLine func(string)) error {
	cmd := proc.Command(ctx, r.Bin, args...)
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
	cmd := proc.Command(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=128x128:d=0.2",
		"-c:v", string(VideoToolbox), "-f", "null", "-")
	if cmd.Run() == nil {
		return VideoToolbox
	}
	return X264
}
