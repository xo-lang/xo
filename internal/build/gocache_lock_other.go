//go:build !unix

package build

// Without flock the cache is not locked; trims fall back to keepRecent
// (gocache.go).

type cacheLock struct{}

func lockCacheShared() *cacheLock             { return nil }
func lockCacheExclusive(wait bool) *cacheLock { return nil }
func (l *cacheLock) unlock()                  {}
