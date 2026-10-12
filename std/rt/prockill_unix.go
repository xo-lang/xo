//go:build !windows && !plan9 && !js && !wasip1

package rt

import "syscall"

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
