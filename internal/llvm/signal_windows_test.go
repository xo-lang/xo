//go:build windows

package llvm_test

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Windows (decision 0197, stage 6) has no SIGTERM: the runtime treats
// Ctrl-C, Ctrl-Break, close, and shutdown console events as the stop
// signal. The child starts in a process group of its own, which a
// Ctrl-Break addressed to that group (its PID) reaches alone; the test
// process gets a console first when it has none (an ssh session), so
// the child shares it and the event can be sent.

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	ctrlEvent    = kernel32.NewProc("GenerateConsoleCtrlEvent")
	getConsole   = kernel32.NewProc("GetConsoleWindow")
	allocConsole = kernel32.NewProc("AllocConsole")
	consoleOnce  sync.Once
)

const ctrlBreak = 1 // CTRL_BREAK_EVENT

func stopAttr(cmd *exec.Cmd) {
	consoleOnce.Do(func() {
		if w, _, _ := getConsole.Call(); w == 0 {
			allocConsole.Call()
		}
	})
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// sendStop sends a Ctrl-Break to our own child's process group.
func sendStop(p *os.Process) error {
	if r, _, err := ctrlEvent.Call(ctrlBreak, uintptr(p.Pid)); r == 0 {
		return os.NewSyscallError("GenerateConsoleCtrlEvent", err)
	}
	return nil
}
