//go:build unix

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// TestCleanCache covers `xo clean -cache [-n]` (2026-10-22): every cache
// in compiler/prune's table under a temporary <user cache>/xo is emptied
// (its stamp and lock files kept), -n removes nothing, an entry whose
// build.lock a live process holds stays, nothing outside <user cache>/xo
// is touched, and `xo clean` alone is a usage error.
func TestCleanCache(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	setenv(t, "HOME", home)
	setenv(t, "XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	cache := filepath.Join(home, ".cache", "xo")
	if runtime.GOOS == "darwin" {
		cache = filepath.Join(home, "Library", "Caches", "xo")
	}
	files := map[string]string{
		"build/0123/bin/prog":       "0123456789",
		"build/prune-stamp":         "",
		"backend/fn/ab/cd":          "x",
		"gocache/00/a-d":            "xx",
		"testbin/00/t":              "xxx",
		"checkout/k1/xo":            "busy",
		"checkout/k1/build.lock":    "",
		"checkout/k2/xo":            "idle",
		"checkout/k2/build.lock":    "",
		"mod/example.com/m@v1/a.xo": "kept",
		"seeds/0.25.0/x":            "kept",
	}
	for name, body := range files {
		p := filepath.Join(cache, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(home, "project", "main.xo")
	os.MkdirAll(filepath.Dir(outside), 0o755)
	os.WriteFile(outside, []byte("fn main(os: Os) {}\n"), 0o644)
	// a build in flight holds checkout/k1's lock
	lf, err := os.OpenFile(filepath.Join(cache, "checkout", "k1", "build.lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	r := xo(t, "clean").want(2)
	if !strings.Contains(r.err, "-cache") {
		t.Errorf("xo clean alone: %s", r.err)
	}
	r = xo(t, "clean", "-cache", "-n").want(0)
	if !strings.Contains(r.out, "would remove 10 B from build") || !strings.Contains(r.out, "would free") {
		t.Errorf("-n:\n%s", r.out)
	}
	if _, err := os.Stat(filepath.Join(cache, "build", "0123", "bin", "prog")); err != nil {
		t.Errorf("-n removed a file: %v", err)
	}
	r = xo(t, "clean", "-cache").want(0)
	if !strings.Contains(r.out, "removed 10 B from build") || !strings.Contains(r.out, "1 in use by a running build") {
		t.Errorf("clean:\n%s", r.out)
	}
	for _, gone := range []string{"build/0123", "backend/fn", "gocache/00", "testbin/00", "checkout/k2"} {
		if _, err := os.Stat(filepath.Join(cache, gone)); err == nil {
			t.Errorf("%s is still there", gone)
		}
	}
	for _, kept := range []string{"build/prune-stamp", "checkout/k1/xo", "mod/example.com/m@v1/a.xo", "seeds/0.25.0/x"} {
		if _, err := os.Stat(filepath.Join(cache, kept)); err != nil {
			t.Errorf("%s was removed: %v", kept, err)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a file outside the cache was removed: %v", err)
	}
}
