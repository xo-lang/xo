//go:build !windows

package llvm

import (
	"os"
	"os/exec"
	"syscall"
)

// stopAttr prepares a child for sendStop (nothing on Unix).
func stopAttr(*exec.Cmd) {}

// sendStop asks our own child to stop: SIGTERM.
func sendStop(p *os.Process) error { return p.Signal(syscall.SIGTERM) }
