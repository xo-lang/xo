//go:build !unix

package gotool

// Without flock the cache is not locked; trims fall back to keepRecent
// (internal/build/gocache.go).

// CacheLock is a held lock on the Xo Go cache; nil means not locked.
type CacheLock struct{}

func LockCacheShared() *CacheLock             { return nil }
func LockCacheExclusive(wait bool) *CacheLock { return nil }
func (l *CacheLock) Unlock()                  {}
