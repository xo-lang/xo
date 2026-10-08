package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Xo builds use their own Go build cache with a size cap. Every generated
// program (and every test copy of one) adds its packages to Go's cache, and
// Go trims its cache only by age, so a night of builds filled 189 GB of the
// default cache (gotchas/go-build-cache-growth.md).

// defaultCacheMax is the cap when XO_GOCACHE_MAX is not set.
const defaultCacheMax = 8 << 30

// keepRecent protects entries Go may be using: Go refreshes an entry's mtime
// only when it is more than an hour old, so anything newer than this may be
// part of a build in flight.
const keepRecent = 2 * time.Hour

// GoCacheDir is the Go build cache used for Xo builds: $XO_GOCACHE, else
// <user cache>/xo/gocache.
func GoCacheDir() string {
	if d := os.Getenv("XO_GOCACHE"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "xo", "gocache")
}

// cacheMax is the cap in bytes: XO_GOCACHE_MAX (bytes), else 8 GiB.
func cacheMax() int64 {
	if v, err := strconv.ParseInt(os.Getenv("XO_GOCACHE_MAX"), 10, 64); err == nil && v > 0 {
		return v
	}
	return defaultCacheMax
}

// trimGoCache removes the least recently used cache files once the cache
// exceeds its cap, down to three quarters of it. It runs at most every ten
// minutes (a stamp file records the last run) so builds stay fast. Go
// refreshes the modification time of an entry it uses only when the entry
// is over an hour old, so mtime order is usage order at one hour
// granularity. Entries touched within keepRecent are never removed, and each
// file is restated just before removal: a concurrent build (another test
// package, another worktree) may be linking it right now
// (gotchas/go-cache-trim-races-builds.md). A removed entry is simply rebuilt.
func trimGoCache() {
	dir := GoCacheDir()
	stamp := filepath.Join(dir, "xo-trim-stamp")
	if fi, err := os.Stat(stamp); err == nil && time.Since(fi.ModTime()) < 10*time.Minute {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(stamp, nil, 0o644)
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var files []entry
	var total int64
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == stamp {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files = append(files, entry{p, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	limit := cacheMax()
	if total <= limit {
		return
	}
	cutoff := time.Now().Add(-keepRecent)
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	target := limit / 4 * 3
	for _, f := range files {
		if total <= target {
			break
		}
		if !f.mod.Before(cutoff) {
			break // sorted oldest first: everything left is recent
		}
		if fi, err := os.Stat(f.path); err != nil || !fi.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
