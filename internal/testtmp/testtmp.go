// Package testtmp makes the temporary directory a test binary shares
// across its tests, and removes the ones earlier runs left behind.
//
// TestMain removes its directory after m.Run returns, but a test binary
// stopped by -timeout, a signal, or os.Exit never reaches that line, and
// each such run left gigabytes of builds in os.TempDir()
// (gotchas/test-temp-dirs-leak-on-killed-runs.md).
package testtmp

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ownerFile holds the PID of the process that owns a directory.
const ownerFile = "owner.pid"

// unownedAge is how old a directory without an owner file must be before
// a sweep removes it: such a directory is either being created right now
// or was made by code from before owner files.
const unownedAge = 24 * time.Hour

// Dir removes the prefix* directories in os.TempDir() whose owner has
// exited, then creates a new one owned by this process.
func Dir(prefix string) (string, error) {
	Sweep(prefix)
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ownerFile), []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// Sweep removes the prefix* directories in os.TempDir() whose owner has
// exited. A directory a running process owns is kept.
func Sweep(prefix string) {
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), prefix+"*"))
	for _, m := range matches {
		if stale(m) {
			os.RemoveAll(m)
		}
	}
}

func stale(dir string) bool {
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	b, err := os.ReadFile(filepath.Join(dir, ownerFile))
	if err != nil {
		return os.IsNotExist(err) && time.Since(fi.ModTime()) > unownedAge
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return false
	}
	return pid != os.Getpid() && !alive(pid)
}
