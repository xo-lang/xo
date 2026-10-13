//go:build windows

package rt

import (
	"os"
	"syscall"
)

// killProcess ends p with the status 137 (128 plus SIGKILL's number),
// so a killed child reads as on Unix (std.md 3.7, decision 0197);
// os.Process.Kill would leave the status 1.
func killProcess(p *os.Process) error {
	h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return p.Kill()
	}
	defer syscall.CloseHandle(h)
	if err := syscall.TerminateProcess(h, 137); err != nil {
		return os.NewSyscallError("TerminateProcess", err)
	}
	return nil
}
