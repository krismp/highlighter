//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// treeKill runs the tool in its own process group and kills the group.
func treeKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
