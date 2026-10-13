package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/gotool"
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
func GoCacheDir() string { return gotool.CacheDir() }

// GoEnv is the environment of a go command run outside a build (tests,
// tools): the process environment with GOWORK=off, -trimpath (so a
// directory's path is not part of the cache key), and GOCACHE set to
// GoCacheDir. Every go command the repo runs uses it or sets GOCACHE
// itself; TestGoCommandsUseXoCache enforces this (decision 0176).
func GoEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOFLAGS=-trimpath", gotool.CacheEnv())
}

// cacheMax is the cap in bytes: XO_GOCACHE_MAX (bytes), else 8 GiB.
func cacheMax() int64 {
	if v, err := strconv.ParseInt(os.Getenv("XO_GOCACHE_MAX"), 10, 64); err == nil && v > 0 {
		return v
	}
	return defaultCacheMax
}

// trimGoCache removes the least recently used cache files once the cache
// exceeds its cap, down to three quarters of it. It runs at most once a
// minute (a stamp file records the last run) so builds stay fast. A
// single test run writes far more than the cap within an hour (each
// generated program adds about 30 MB: every package instantiates the
// runtime's generics again), so the trim must be able to remove recent
// entries: it takes the cache lock exclusively
// (internal/gotool/cachelock_unix.go), which waits for running go
// commands (Xo builds and every other go command on the cache, which hold
// gotool.HoldCache) and holds new ones off, and then removes any entry. It tries the lock without waiting, and waits for it
// only once the cache is over twice its cap. Without the lock (another
// build is running, or no flock) it removes only entries older than
// keepRecent, restating each file first: Go refreshes an entry's mtime
// only when it is over an hour old, so a build in flight that holds no
// lock (an older xo binary) may be using anything newer
// (gotchas/go-cache-trim-races-builds.md). A removed entry is simply
// rebuilt.
func trimGoCache() {
	dir := GoCacheDir()
	stamp := filepath.Join(dir, "xo-trim-stamp")
	if fi, err := os.Stat(stamp); err == nil && time.Since(fi.ModTime()) < time.Minute {
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
		if err != nil || d.IsDir() || filepath.Dir(p) == dir && strings.HasPrefix(d.Name(), "xo-") {
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
	lock := gotool.LockCacheExclusive(total > 2*limit)
	defer lock.Unlock()
	cutoff := time.Now().Add(-keepRecent)
	if lock != nil {
		cutoff = time.Now().Add(time.Hour) // no build is running: anything may go
	}
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
