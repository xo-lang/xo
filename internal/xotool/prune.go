package xotool

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Pruning of <cache base>/checkout/<key> (decision 0223), the xo builds
// Ensure shares across worktrees: the stamp format of selfhost/prune
// (an entry's last use is its modification time, refreshed at most
// hourly; the last prune is the time of checkout/prune-stamp). At most
// once an hour, the keys beyond the newest $XO_CHECKOUT_KEEP (default 4)
// that were not used within $XO_CACHE_MAX_AGE (default 24h; 0 turns the
// pruning off) are removed. The key being used and any key used within
// two hours (a build in flight holds build.lock in its directory for
// minutes) always stay. Five agents left 5 GB of them in a day
// (2026-10-14), about 80 MB each.

const (
	pruneTouch = time.Hour     // markUsed refreshes a time older than this
	pruneKeep  = 2 * time.Hour // never remove a key used within this
	pruneEvery = time.Hour     // the least time between two prunes
)

// pruneCfg is what a prune of the checkout cache keeps.
type pruneCfg struct {
	keep   int           // the newest keys kept whatever their age
	maxAge time.Duration // older keys unused for longer go; 0 keeps all
	every  time.Duration
}

// checkoutPruneCfg reads $XO_CHECKOUT_KEEP and $XO_CACHE_MAX_AGE.
func checkoutPruneCfg() pruneCfg {
	c := pruneCfg{keep: 4, maxAge: 24 * time.Hour, every: pruneEvery}
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
	for i, k := range keys {
		idle := now.Sub(k.used)
		if i < c.keep || k.name == current || idle <= c.maxAge || idle < pruneKeep {
			continue
		}
		p := filepath.Join(dir, k.name)
		// restat: another process may have used it since the listing
		if fi, err := os.Stat(p); err != nil || now.Sub(fi.ModTime()) <= c.maxAge || now.Sub(fi.ModTime()) < pruneKeep {
			continue
		}
		if os.RemoveAll(p) == nil {
			removed = append(removed, k.name)
		}
	}
	return removed
}

// useCheckout marks base/checkout/key used and prunes base/checkout.
func useCheckout(base, key string) {
	dir := filepath.Join(base, "checkout")
	now := time.Now()
	markUsed(filepath.Join(dir, key), now)
	pruneCheckouts(dir, key, now, checkoutPruneCfg())
}
