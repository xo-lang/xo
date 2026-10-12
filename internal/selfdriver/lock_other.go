//go:build !unix

package selfdriver

// lockFile does not lock without flock; the native backends (and so xo1)
// run on darwin/arm64 and linux/arm64 hosts only (Wants).
func lockFile(path string) (func(), error) { return func() {}, nil }
