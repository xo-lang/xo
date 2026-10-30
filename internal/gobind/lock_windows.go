//go:build windows

package gobind

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// lockFile takes an exclusive LockFileEx lock on the first byte of path
// (created if missing), waiting for it, and returns its release; the
// system drops it if the process dies.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	var ol syscall.Overlapped
	const lockfileExclusiveLock = 0x2
	if r, _, err := procLockFileEx.Call(f.Fd(), lockfileExclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&ol))); r == 0 {
		f.Close()
		return nil, err
	}
	return func() {
		var ol syscall.Overlapped
		procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ol)))
		f.Close()
	}, nil
}
