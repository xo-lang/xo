//go:build !unix && !windows

package gobind

// lockFile does not lock where the platform has no file locks (wasip1).
func lockFile(path string) (func(), error) { return func() {}, nil }
