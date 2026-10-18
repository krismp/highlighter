// Package proc starts external tools so that cancelling their context stops
// the whole process tree, not just the direct child: yt-dlp is a
// self-extracting launcher that runs Python, which can run deno.
package proc

import (
	"context"
	"os/exec"
	"time"
)

// Command is exec.CommandContext whose cancellation kills the process and
// everything it started.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	treeKill(cmd)
	cmd.WaitDelay = 5 * time.Second // don't hang on pipes held open by a killed tree
	return cmd
}
