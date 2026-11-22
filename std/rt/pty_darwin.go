//go:build darwin

package rt

import (
	"os"
	"syscall"
)

// ptyOpenMaster opens /dev/ptmx outside the runtime poller: kqueue does
// not report a master's end once the slave's last holder closes it
// (select does), so ptyRead waits in select(2) (gotchas/
// pty-darwin-master-and-ctty.md).
func ptyOpenMaster() (*os.File, error) {
	fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "/dev/ptmx"), nil
}

// ptyTraceExec and ptyRelease: the parent's slave closes once Start
// returns (pty_freebsd.go stops the child at its exec first).
func ptyTraceExec(*syscall.SysProcAttr) {}

func ptyRelease(s *os.File, _ int) { s.Close() }
