//go:build freebsd

package rt

import (
	"os"
	"syscall"
	"unsafe"
)

// ptyOpenMaster opens a master with posix_openpt(2), outside the runtime
// poller as on macOS (pty_bsd.go's ptyRead waits in select(2)); FreeBSD's
// grantpt and unlockpt do nothing, and TIOCGPTN names the slave.
func ptyOpenMaster() (*os.File, error) {
	fd, _, e := syscall.Syscall(syscall.SYS_POSIX_OPENPT, uintptr(syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC), 0, 0)
	if e != 0 {
		return nil, e
	}
	return os.NewFile(fd, "/dev/ptmx"), nil
}

// ptyTraceExec stops the child at its exec (PT_TRACE_ME) so ptyRelease can
// close the parent's slave before the child runs: FreeBSD revokes a
// session's terminal when its leader exits, and the revoke discards the
// output the master had not read while another descriptor (the parent's
// copy) still holds the slave; the leader's own close as the last one
// keeps it. Go's Start returns well after the exec, and a short child (stty
// -a) had exited by then in 8 of 40 runs on a FreeBSD VM
// (gotchas/pty-darwin-master-and-ctty.md).
func ptyTraceExec(a *syscall.SysProcAttr) { a.Ptrace = true }

// ptyRelease closes s, the parent's slave, while pid sits stopped at its
// exec, then detaches pid (PT_DETACH 11, continuing where it stopped).
// wait6(2) first peeks (WNOWAIT) so a child killed meanwhile is left for
// Wait; then it takes the trace stop (WTRAPPED 32), which PT_DETACH needs.
func ptyRelease(s *os.File, pid int) {
	stopped := false
	if pid > 0 {
		var st syscall.WaitStatus
		var info [128]byte
		for {
			_, _, e := syscall.Syscall6(syscall.SYS_WAIT6, 0, uintptr(pid), uintptr(unsafe.Pointer(&st)), 32|16|8, 0, uintptr(unsafe.Pointer(&info[0])))
			if e != syscall.EINTR {
				stopped = e == 0 && st.Stopped()
				break
			}
		}
		if stopped {
			for {
				_, _, e := syscall.Syscall6(syscall.SYS_WAIT6, 0, uintptr(pid), uintptr(unsafe.Pointer(&st)), 32, 0, 0)
				if e != syscall.EINTR {
					break
				}
			}
		}
	}
	s.Close()
	if stopped {
		syscall.Syscall6(syscall.SYS_PTRACE, 11, uintptr(pid), 1, 0, 0, 0)
	}
}
