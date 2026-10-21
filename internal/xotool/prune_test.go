package xotool

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// mkKey makes dir/name with an xo file, its time set to at.
func mkKey(t *testing.T, dir, name string, at time.Time) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "xo"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func keysIn(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// TestPruneCheckouts: past the newest keep keys, a key unused for longer
// than maxAge goes; the current key, keys used within the age, and the
// newest keep stay; the stamp limits prunes to one an hour. The clock is
// a fixed time, no sleeps.
func TestPruneCheckouts(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	mkKey(t, dir, "a", now.Add(-10*day))       // oldest, stale: goes
	mkKey(t, dir, "b", now.Add(-9*day))        // stale but current: stays
	mkKey(t, dir, "c", now.Add(-8*day))        // stale: goes
	mkKey(t, dir, "d", now.Add(-20*time.Hour)) // within the age: stays
	mkKey(t, dir, "e", now.Add(-7*day))        // stale: goes
	mkKey(t, dir, "f", now.Add(-1*time.Hour))
	mkKey(t, dir, "g", now.Add(-2*time.Hour))
	c := pruneCfg{keep: 2, maxAge: day, every: time.Hour}
	got := pruneCheckouts(dir, "b", now, c)
	sort.Strings(got)
	if want := []string{"a", "c", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed %v, want %v", got, want)
	}
	if want := []string{"b", "d", "f", "g"}; !reflect.DeepEqual(keysIn(t, dir), want) {
		t.Fatalf("left %v, want %v", keysIn(t, dir), want)
	}
	// within the hour the stamp skips the prune
	mkKey(t, dir, "h", now.Add(-30*day))
	if got := pruneCheckouts(dir, "b", now.Add(30*time.Minute), c); got != nil {
		t.Fatalf("prune within the hour removed %v", got)
	}
	if got := pruneCheckouts(dir, "b", now.Add(2*time.Hour), c); !reflect.DeepEqual(got, []string{"h"}) {
		t.Fatalf("prune after the hour removed %v, want [h]", got)
	}
}

// TestPruneCheckoutsInUse: a key marked used (Ensure's markUsed) is not
// removed even when it is past the newest keep and its build is old, and
// a zero age turns pruning off.
func TestPruneCheckoutsInUse(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	mkKey(t, dir, "new", now)
	mkKey(t, dir, "busy", now.Add(-48*time.Hour))
	mkKey(t, dir, "idle", now.Add(-49*time.Hour))
	if got := pruneCheckouts(dir, "", now, pruneCfg{keep: 1, maxAge: 0, every: time.Hour}); got != nil {
		t.Fatalf("a zero age removed %v", got)
	}
	markUsed(filepath.Join(dir, "busy"), now)
	got := pruneCheckouts(dir, "", now, pruneCfg{keep: 1, maxAge: 24 * time.Hour, every: time.Hour})
	if !reflect.DeepEqual(got, []string{"idle"}) {
		t.Fatalf("removed %v, want [idle]", got)
	}
	// markUsed within pruneTouch leaves the time (one stat)
	at := now.Add(-5 * time.Minute)
	os.Chtimes(filepath.Join(dir, "new"), at, at)
	markUsed(filepath.Join(dir, "new"), now)
	if fi, _ := os.Stat(filepath.Join(dir, "new")); !fi.ModTime().Equal(at) {
		t.Fatalf("markUsed refreshed a time 5 minutes old")
	}
}

func TestCheckoutPruneCfg(t *testing.T) {
	t.Setenv("XO_CHECKOUT_KEEP", "7")
	t.Setenv("XO_CACHE_MAX_AGE", "6h")
	if c := checkoutPruneCfg(); c.keep != 7 || c.maxAge != 6*time.Hour {
		t.Fatalf("got %+v", c)
	}
	t.Setenv("XO_CHECKOUT_KEEP", "")
	t.Setenv("XO_CACHE_MAX_AGE", "0")
	if c := checkoutPruneCfg(); c.keep != 4 || c.maxAge != 0 {
		t.Fatalf("got %+v", c)
	}
}

// TestPruneCheckoutsSizeCap: past the newest keep, the least recently
// used keys go while the keys hold more than maxBytes, all of them
// younger than maxAge; a key used within 30 minutes and a key whose
// build.lock is held stay (2026-10-21: 385 keys, 26 GB, none a day old).
func TestPruneCheckoutsSizeCap(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)
	big := make([]byte, 1000)
	mk := func(name string, ago time.Duration) {
		mkKey(t, dir, name, now.Add(-ago))
		if err := os.WriteFile(filepath.Join(dir, name, "xo"), big, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dir, name), now.Add(-ago), now.Add(-ago)); err != nil {
			t.Fatal(err)
		}
	}
	mk("newest", time.Minute)
	mk("recent", 10*time.Minute) // within 30 minutes: stays
	mk("locked", 5*time.Hour)    // a build in flight: stays
	mk("old1", 3*time.Hour)
	mk("old2", 4*time.Hour)
	mk("old3", 6*time.Hour) // the least recently used
	unlock, err := lockFile(filepath.Join(dir, "locked", "build.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	// 6,000 bytes against a cap of 3,500: three of the removable go,
	// oldest first, and the locked one is passed over
	got := pruneCheckouts(dir, "", now, pruneCfg{keep: 1, maxAge: 24 * time.Hour, every: time.Hour, maxBytes: 3500})
	sort.Strings(got)
	if want := []string{"old1", "old2", "old3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed %v, want %v", got, want)
	}
	if want := []string{"locked", "newest", "recent"}; !reflect.DeepEqual(keysIn(t, dir), want) {
		t.Fatalf("left %v, want %v", keysIn(t, dir), want)
	}
}
