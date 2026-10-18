//go:build !windows

package proc

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A shell that starts a grandchild: cancelling must kill the grandchild too.
func TestCancelKillsTheWholeTree(t *testing.T) {
	pidFile := t.TempDir() + "/grandchild.pid"
	ctx, cancel := context.WithCancel(context.Background())
	cmd := Command(ctx, "sh", "-c", "sleep 60 & echo $! > "+pidFile+"; wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(50 * time.Millisecond)
		data, _ := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if pid == 0 {
		t.Fatal("grandchild did not start")
	}
	cancel()
	cmd.Wait()
	for i := 0; i < 50; i++ {
		if syscall.Kill(pid, 0) != nil {
			return // gone
		}
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Fatal("grandchild survived cancellation")
}
