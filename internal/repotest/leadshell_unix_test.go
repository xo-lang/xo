//go:build unix

package repotest

import (
	"os/exec"
	"syscall"
)

// leadShellGroup starts cmd in its own process group and makes its
// cancel signal the whole group (TERM, then KILL after WaitDelay).
func leadShellGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

// leadShellReap stops what the script left running in its group.
func leadShellReap(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}
