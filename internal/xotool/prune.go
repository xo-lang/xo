package xotool

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Pruning of <cache base>/checkout/<key> (decision 0223), the xo builds
// Ensure shares across worktrees: the stamp format of compiler/prune
// (an entry's last use is its modification time, refreshed at most
// hourly; the last prune is the time of checkout/prune-stamp). At most
// once an hour, the keys beyond the newest $XO_CHECKOUT_KEEP (default 4)
// that were not used within $XO_CACHE_MAX_AGE (default 24h; 0 turns the
// pruning off) are removed, then the least recently used beyond those
// newest while the keys hold more than $XO_CHECKOUT_MAX bytes (default
// 8 GiB; 2026-10-21: 385 keys, 26 GB, all younger than a day). The key
// being used, any key used within 30 minutes, and any key whose
// build.lock is held (a build in flight) always stay. Five agents left 5
// GB of them in a day (2026-10-14), about 80 MB each.

const (
	pruneTouch = 10 * time.Minute // markUsed refreshes a time older than this
	pruneKeep  = 30 * time.Minute // never remove a key used within this
	pruneEvery = time.Hour        // the least time between two prunes
)

// pruneCfg is what a prune of the checkout cache keeps.
type pruneCfg struct {
	keep     int           // the newest keys kept whatever their age
	maxAge   time.Duration // older keys unused for longer go; 0 keeps all
	every    time.Duration
	maxBytes int64 // over it, the least recently used go; 0: no cap
}

// checkoutPruneCfg reads $XO_CHECKOUT_KEEP and $XO_CACHE_MAX_AGE.
func checkoutPruneCfg() pruneCfg {
	c := pruneCfg{keep: 4, maxAge: 24 * time.Hour, every: pruneEvery, maxBytes: 8 << 30}
	if n, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("XO_CHECKOUT_MAX")), 10, 64); err == nil && n >= 0 {
		c.maxBytes = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("XO_CHECKOUT_KEEP"))); err == nil && n >= 0 {
		c.keep = n
	}
	if v := strings.TrimSpace(os.Getenv("XO_CACHE_MAX_AGE")); v != "" {
		if v == "0" {
			c.maxAge = 0
		} else if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			c.maxAge = d
		}
	}
	return c
}

// markUsed sets path's modification time to now when it is over
// pruneTouch old: one stat otherwise.
func markUsed(path string, now time.Time) {
	if fi, err := os.Stat(path); err == nil && now.Sub(fi.ModTime()) >= pruneTouch {
		_ = os.Chtimes(path, now, now)
	}
}

// pruneCheckouts prunes the key directories of dir (base/checkout) by c
// at time now, keeping current, and returns the keys it removed.
func pruneCheckouts(dir, current string, now time.Time, c pruneCfg) []string {
	if c.maxAge <= 0 {
		return nil
	}
	stamp := filepath.Join(dir, "prune-stamp")
	if fi, err := os.Stat(stamp); err == nil && now.Sub(fi.ModTime()) < c.every {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	if os.WriteFile(stamp, nil, 0o644) == nil {
		_ = os.Chtimes(stamp, now, now)
	}
	type key struct {
		name string
		used time.Time
	}
	var keys []key
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if fi, err := e.Info(); err == nil {
			keys = append(keys, key{e.Name(), fi.ModTime()})
		}
	}
	// newest first, by name on equal times so the order is stable
	sort.Slice(keys, func(i, j int) bool {
		if !keys[i].used.Equal(keys[j].used) {
			return keys[i].used.After(keys[j].used)
		}
		return keys[i].name < keys[j].name
	})
	var removed []string
	gone := map[string]bool{}
	// removable: not among the newest, not the one in use, idle past the
	// keep (restated: another process may have used it since the
	// listing), and no build in flight in it
	removable := func(i int, k key, age time.Duration) bool {
		if i < c.keep || k.name == current {
			return false
		}
		p := filepath.Join(dir, k.name)
		fi, err := os.Stat(p)
		if err != nil {
			return false
		}
		idle := now.Sub(fi.ModTime())
		return idle > age && idle >= pruneKeep && !lockHeld(filepath.Join(p, "build.lock"))
	}
	for i, k := range keys {
		if removable(i, k, c.maxAge) && os.RemoveAll(filepath.Join(dir, k.name)) == nil {
			removed = append(removed, k.name)
			gone[k.name] = true
		}
	}
	if c.maxBytes <= 0 {
		return removed
	}
	sizes := map[string]int64{}
	var total int64
	for _, k := range keys {
		if gone[k.name] {
			continue
		}
		n := dirBytes(filepath.Join(dir, k.name))
		sizes[k.name] = n
		total += n
	}
	// the least recently used first, while over the cap
	for i := len(keys) - 1; i >= 0 && total > c.maxBytes; i-- {
		k := keys[i]
		if gone[k.name] || !removable(i, k, 0) {
			continue
		}
		if os.RemoveAll(filepath.Join(dir, k.name)) == nil {
			removed = append(removed, k.name)
			gone[k.name] = true
			total -= sizes[k.name]
		}
	}
	return removed
}

// dirBytes is the size of the files below p.
func dirBytes(p string) int64 {
	var n int64
	_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// useCheckout marks base/checkout/key used and prunes base/checkout.
func useCheckout(base, key string) {
	dir := filepath.Join(base, "checkout")
	now := time.Now()
	markUsed(filepath.Join(dir, key), now)
	pruneCheckouts(dir, key, now, checkoutPruneCfg())
}
