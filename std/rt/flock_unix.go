//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package rt

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile takes an exclusive flock(2) lock on f without waiting; false
// when another holder has one.
func tryLockFile(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		}
		return false, err
	}
}

func unlockFile(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
