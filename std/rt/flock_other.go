//go:build !windows && !darwin && !linux && !freebsd && !netbsd && !openbsd && !dragonfly

package rt

import "os"

// No file locks on this target (wasip1, js, plan9): a lock is always
// granted. The program runs alone there.
func tryLockFile(*os.File) (bool, error) { return true, nil }
func unlockFile(*os.File)                {}
