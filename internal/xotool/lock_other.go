//go:build !unix

package xotool

// lockFile does not lock without flock (xo builds itself on darwin and
// linux hosts).
func lockFile(path string) (func(), error) { return func() {}, nil }

// lockHeld is false without flock.
func lockHeld(path string) bool { return false }
