package build

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// writeAged writes 60 byte cache files with the given ages.
func writeAged(t *testing.T, dir string, ages map[string]time.Duration) {
	t.Helper()
	now := time.Now()
	for name, age := range ages {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, 60), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := now.Add(-age)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
}

func checkKept(t *testing.T, dir string, want map[string]bool) {
	t.Helper()
	for name, kept := range want {
		_, err := os.Stat(filepath.Join(dir, name))
		if got := err == nil; got != kept {
			t.Errorf("%s: kept=%v, want %v", name, got, kept)
		}
	}
}

var agedFiles = map[string]time.Duration{
	"old1": 5 * time.Hour, "old2": 4 * time.Hour,
	"recent1": 30 * time.Minute, "recent2": time.Minute,
}

// A build holding the cache lock keeps the trim to entries older than
// keepRecent (the cache is under twice its cap, so the trim does not wait
// for the lock).
func TestTrimGoCacheKeepsRecentDuringBuild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XO_GOCACHE", dir)
	t.Setenv("XO_GOCACHE_MAX", "150")
	writeAged(t, dir, agedFiles)
	build := lockCacheShared()
	trimGoCache()
	build.unlock()
	checkKept(t, dir, map[string]bool{"old1": false, "old2": false, "recent1": true, "recent2": true})
}

// With no build running the trim takes the lock and removes recent
// entries too, oldest first, down to three quarters of the cap.
func TestTrimGoCacheRemovesRecentWhenIdle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no cache lock without flock")
	}
	dir := t.TempDir()
	t.Setenv("XO_GOCACHE", dir)
	t.Setenv("XO_GOCACHE_MAX", "150")
	writeAged(t, dir, agedFiles)
	trimGoCache()
	checkKept(t, dir, map[string]bool{"old1": false, "old2": false, "recent1": false, "recent2": true})
}

// A trim waiting for the lock (cache over twice its cap) runs once the
// build releases it.
func TestTrimGoCacheWaitsForBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no cache lock without flock")
	}
	dir := t.TempDir()
	t.Setenv("XO_GOCACHE", dir)
	t.Setenv("XO_GOCACHE_MAX", "100")
	writeAged(t, dir, map[string]time.Duration{"a": time.Minute, "b": 2 * time.Minute, "c": 3 * time.Minute, "d": 4 * time.Minute})
	build := lockCacheShared()
	done := make(chan struct{})
	go func() { trimGoCache(); close(done) }()
	select {
	case <-done:
		t.Fatal("trim did not wait for the build's lock")
	case <-time.After(200 * time.Millisecond):
	}
	build.unlock()
	<-done
	checkKept(t, dir, map[string]bool{"a": true, "b": false, "c": false, "d": false})
}
