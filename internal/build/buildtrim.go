package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// Build directories (<user cache>/xo/build/<hash>, one per program and
// mode) hold the generated Go and the runtime copy; nothing removed them,
// so test runs and worktree agents left 21 GB of them (3,066 directories)
// by 2026-10-10 (gotchas/build-dir-growth.md). trimBuildDirs removes the
// least recently used directories once the total exceeds
// XO_BUILD_DIR_MAX (bytes, default 4 GiB), down to three quarters of it.
// A directory used within keepRecent is never removed (a build may be
// writing it); buildDir refreshes a directory's mtime on each use. It runs
// at most once a minute, recorded by a stamp file.

// buildDirMaxDefault is the cap when XO_BUILD_DIR_MAX is not set.
const buildDirMaxDefault = 4 << 30

func buildDirMax() int64 {
	if v, err := strconv.ParseInt(os.Getenv("XO_BUILD_DIR_MAX"), 10, 64); err == nil && v > 0 {
		return v
	}
	return buildDirMaxDefault
}

func trimBuildDirs(base string) {
	stamp := filepath.Join(base, ".xo-trim-stamp")
	if fi, err := os.Stat(stamp); err == nil && time.Since(fi.ModTime()) < time.Minute {
		return
	}
	_ = os.WriteFile(stamp, nil, 0o644)
	ents, err := os.ReadDir(base)
	if err != nil {
		return
	}
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	var dirs []entry
	var total int64
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(base, e.Name())
		var size int64
		_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if fi, err := d.Info(); err == nil {
					size += fi.Size()
				}
			}
			return nil
		})
		dirs = append(dirs, entry{p, size, info.ModTime()})
		total += size
	}
	limit := buildDirMax()
	if total <= limit {
		return
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].mod.Before(dirs[j].mod) })
	cutoff := time.Now().Add(-keepRecent)
	target := limit / 4 * 3
	for _, d := range dirs {
		if total <= target || !d.mod.Before(cutoff) {
			break
		}
		if fi, err := os.Stat(d.path); err != nil || !fi.ModTime().Before(cutoff) {
			continue
		}
		if os.RemoveAll(d.path) == nil {
			total -= d.size
		}
	}
}
