package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// TestCacheStats: XO_CACHE_STATS=1 ends a build with a summary of its
// cache file operations on standard error: a cold build writes cache
// entries, a warm one reads them, and the total counts them all.
func TestCacheStats(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	tmp := t.TempDir()
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", "off")
	setenv(t, "XO_CACHE_STATS", "1")
	dir := filepath.Join(tmp, "prog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module stats\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	total := regexp.MustCompile(`(?m)^cache stats: total: ([1-9][0-9]*) calls, ([0-9]+) bytes$`)
	for _, step := range []struct{ name, want string }{
		{"cold", `(?m)^cache stats: (entry|shared|pack) write: [1-9]`},
		{"warm", `(?m)^cache stats: (entry|shared|pack) read hit: [1-9]`},
	} {
		var so, se bytes.Buffer
		if code := run(t, []string{"build", "-o", filepath.Join(dir, "prog"), "."}, &so, &se); code != 0 {
			t.Fatalf("%s build: exit %d\n%s", step.name, code, se.String())
		}
		if !total.MatchString(se.String()) || !regexp.MustCompile(step.want).MatchString(se.String()) {
			t.Fatalf("%s build: want a total and %s in\n%s", step.name, step.want, se.String())
		}
		// a warm build reads back what the cold one stored (about 20 MB for
		// hello, from loose entries or the shared pack's segments): a build
		// whose lookups were all skipped passes the cold build's "under 100
		// misses" below, as one under XO_CACHE_STATS did while the stats Fs
		// was refused by internal.dir_names (2026-10-24)
		if step.name == "warm" {
			read := 0
			for _, m := range regexp.MustCompile(`(?m)^cache stats: (?:entry|pack|shared) read hit: [0-9]+ calls, ([0-9]+) bytes`).FindAllStringSubmatch(se.String(), -1) {
				n, _ := strconv.Atoi(m[1])
				read += n
			}
			if read < 1_000_000 {
				t.Fatalf("warm build: %d bytes read back, want 1 MB or more\n%s", read, se.String())
			}
		}
		// a cold build does not open entries the cache cannot hold
		// (bcache.xo cache_get_all lists the shards: 21,413 failed opens
		// for a cold CRUD build before, 2026-10-23)
		if m := regexp.MustCompile(`(?m)^cache stats: entry read miss: ([0-9]+) calls`).FindStringSubmatch(se.String()); step.name == "cold" && m != nil && len(m[1]) > 2 {
			t.Fatalf("cold build: %s entry reads missed, want under 100\n%s", m[1], se.String())
		}
	}
}
