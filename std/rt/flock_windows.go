//go:build windows

package rt

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x2
	lockfileFailImmediately = 0x1
	errorLockViolation      = syscall.Errno(33)
)

// tryLockFile takes an exclusive (shared when shared: no
// LOCKFILE_EXCLUSIVE_LOCK) lock on the first byte of f without waiting;
// false when another holder has a conflicting one.
func tryLockFile(f *os.File, shared bool) (bool, error) {
	var ol syscall.Overlapped
	flags := uintptr(lockfileExclusiveLock | lockfileFailImmediately)
	if shared {
		flags = lockfileFailImmediately
	}
	r, _, err := procLockFileEx.Call(f.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
	if r != 0 {
		return true, nil
	}
	if err == errorLockViolation || err == syscall.ERROR_IO_PENDING {
		return false, nil
	}
	return false, err
}

func unlockFile(f *os.File) {
	var ol syscall.Overlapped
	procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
}
