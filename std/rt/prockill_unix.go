//go:build !windows && !plan9 && !js && !wasip1

package rt

import (
	"os/exec"
	"syscall"
)

var signalNums = map[string]syscall.Signal{
	"HUP": syscall.SIGHUP, "INT": syscall.SIGINT, "QUIT": syscall.SIGQUIT, "KILL": syscall.SIGKILL,
	"TERM": syscall.SIGTERM, "USR1": syscall.SIGUSR1, "USR2": syscall.SIGUSR2, "CONT": syscall.SIGCONT,
	"STOP": syscall.SIGSTOP,
}

// sendSignal is kill(2); its error is the errno's text ("no such
// process", "operation not permitted").
func sendSignal(pid int, sig string) error {
	return syscall.Kill(pid, signalNums[sig])
}

// groupsSupported: Cmd.new_group makes a process group (decision 0739).
const groupsSupported = true

// setGroup starts x's process as the leader of a new process group.
func setGroup(x *exec.Cmd) {
	if x.SysProcAttr == nil {
		x.SysProcAttr = &syscall.SysProcAttr{}
	}
	x.SysProcAttr.Setpgid = true
}
