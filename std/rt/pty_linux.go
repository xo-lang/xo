//go:build linux && (amd64 || arm64 || riscv64)

package rt

import (
	"os"
	"syscall"
	"time"
)

// ptyOpenMaster opens /dev/ptmx through the runtime poller (epoll
// reports the end, EIO once no process holds the slave).
func ptyOpenMaster() (*os.File, error) {
	return os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
}

func ptySetDeadline(m *os.File, at time.Time) { m.SetReadDeadline(at) }

func ptyRead(tk *Task, m *os.File, buf []byte, dl time.Time) (int, bool, error) {
	var n int
	canceled, err := interruptibleTo(tk, m.SetReadDeadline, dl, func() error {
		var e error
		n, e = m.Read(buf)
		return e
	})
	return n, canceled, err
}

// ptyTraceExec and ptyRelease: the parent's slave closes once Start
// returns (pty_freebsd.go stops the child at its exec first).
func ptyTraceExec(*syscall.SysProcAttr) {}

func ptyRelease(s *os.File, _ int) { s.Close() }
