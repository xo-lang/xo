//go:build unix

package testtmp

import (
	"os"
	"syscall"
)

// killTree sends SIGKILL to the process group p leads (Guard's lifeline
// wrapper makes the program a group leader), then to p itself, which
// is all there is without the wrapper.
func killTree(p *os.Process) {
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
	_ = p.Kill()
}
