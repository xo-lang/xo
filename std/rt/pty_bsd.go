//go:build darwin || freebsd

package rt

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

func ptySetDeadline(*os.File, time.Time) {}

// ptyRead (macOS and FreeBSD; ptyOpenMaster keeps the master off the
// runtime poller there) waits for the master in 50 ms select slices, so a deadline and
// the task's cancellation end it, then reads.
func ptyRead(tk *Task, m *os.File, buf []byte, dl time.Time) (int, bool, error) {
	fd := int(m.Fd())
	for {
		if tk.Canceled() {
			return 0, true, nil
		}
		wait := 50 * time.Millisecond
		if !dl.IsZero() {
			left := time.Until(dl)
			if left <= 0 {
				return 0, false, os.ErrDeadlineExceeded
			}
			wait = min(wait, left)
		}
		var set syscall.FdSet
		b := unsafe.Slice((*byte)(unsafe.Pointer(&set)), unsafe.Sizeof(set))
		b[fd/8] |= 1 << (fd % 8)
		tv := syscall.NsecToTimeval(int64(wait))
		if err := syscall.Select(fd+1, &set, nil, nil, &tv); err != nil {
			if err == syscall.EINTR {
				continue
			}
			return 0, false, err
		}
		if b[fd/8]&(1<<(fd%8)) == 0 {
			continue
		}
		n, err := syscall.Read(fd, buf)
		if err == syscall.EINTR || err == syscall.EAGAIN {
			continue
		}
		if n < 0 {
			n = 0
		}
		return n, false, err
	}
}
