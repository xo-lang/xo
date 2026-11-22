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
	// each wait and read runs under the file's reference (Control), so a
	// close from another task waits for it and the next turn sees
	// os.ErrClosed, never the descriptor's number reused by another file
	rc, err := m.SyscallConn()
	if err != nil {
		return 0, false, err
	}
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
		var n int
		var rerr error
		ready := false
		cerr := rc.Control(func(u uintptr) {
			fd := int(u)
			var set syscall.FdSet
			b := unsafe.Slice((*byte)(unsafe.Pointer(&set)), unsafe.Sizeof(set))
			b[fd/8] |= 1 << (fd % 8)
			tv := syscall.NsecToTimeval(int64(wait))
			if err := syscall.Select(fd+1, &set, nil, nil, &tv); err != nil {
				if err != syscall.EINTR {
					ready, rerr = true, err
				}
				return
			}
			if b[fd/8]&(1<<(fd%8)) == 0 {
				return
			}
			n, rerr = syscall.Read(fd, buf)
			ready = rerr != syscall.EINTR && rerr != syscall.EAGAIN
		})
		if cerr != nil {
			// Control fails only on a closed file, with internal/poll's
			// error, which is not os.ErrClosed
			return 0, false, os.ErrClosed
		}
		if !ready {
			continue
		}
		if n < 0 {
			n = 0
		}
		return n, false, rerr
	}
}
