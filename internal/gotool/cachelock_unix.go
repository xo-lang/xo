//go:build unix

package gotool

import (
	"os"
	"path/filepath"
	"syscall"
)

// Cache locks (gotchas/go-cache-trim-races-builds.md,
// gotchas/go-commands-outside-build-race-trim.md). Every go command on
// the Xo Go cache holds a shared lock on xo-cache.lock for its whole run
// (HoldCache); internal/build's trim holds it exclusively, so it may
// remove any entry. A trim first takes xo-gate.lock exclusively, and go
// commands pass through the gate (shared) before locking the cache, so a
// waiting trim is not starved by a stream of new ones.

// CacheLock is a held lock on the Xo Go cache; nil means not locked.
type CacheLock struct{ gate, cache *os.File }

func openLock(name string) (*os.File, error) {
	dir := CacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE, 0o644)
}

func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if err != syscall.EINTR {
			return err
		}
	}
}

// LockCacheShared is held by a go command. On any error the command runs
// unlocked (nil lock), as before locks existed.
func LockCacheShared() *CacheLock {
	gate, err := openLock("xo-gate.lock")
	if err != nil {
		return nil
	}
	cache, err := openLock("xo-cache.lock")
	if err != nil {
		gate.Close()
		return nil
	}
	if flock(gate, syscall.LOCK_SH) != nil || flock(cache, syscall.LOCK_SH) != nil {
		gate.Close()
		cache.Close()
		return nil
	}
	flock(gate, syscall.LOCK_UN)
	gate.Close()
	return &CacheLock{cache: cache}
}

// LockCacheExclusive is held by a trim. wait false gives up at once when
// a go command holds the cache (nil result).
func LockCacheExclusive(wait bool) *CacheLock {
	gate, err := openLock("xo-gate.lock")
	if err != nil {
		return nil
	}
	cache, err := openLock("xo-cache.lock")
	if err != nil {
		gate.Close()
		return nil
	}
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	if flock(gate, how) != nil {
		gate.Close()
		cache.Close()
		return nil
	}
	if flock(cache, how) != nil {
		flock(gate, syscall.LOCK_UN)
		gate.Close()
		cache.Close()
		return nil
	}
	return &CacheLock{gate: gate, cache: cache}
}

// Unlock releases the lock; nil is a no-op.
func (l *CacheLock) Unlock() {
	if l == nil {
		return
	}
	if l.cache != nil {
		flock(l.cache, syscall.LOCK_UN)
		l.cache.Close()
	}
	if l.gate != nil {
		flock(l.gate, syscall.LOCK_UN)
		l.gate.Close()
	}
}
