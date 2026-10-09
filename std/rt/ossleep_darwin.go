package rt

import (
	"syscall"
	"time"
)

// osSleep sleeps in a system call, without a Go runtime timer, so the
// runtime's own watchers never look like a pending Go timer to the
// deadlock proof (gotimers.go).
func osSleep(d time.Duration) {
	tv := syscall.NsecToTimeval(max(d.Nanoseconds(), 1000))
	syscall.Select(0, nil, nil, nil, &tv)
}
