package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
)

// TestHeapProfileRegionBuild is task #658: xo builds a large program
// with the heap profile sampling every block (XO_PROF=heap
// XO_PROF_HEAP_RATE=1) and exits 0 each time. The front end's blocks
// live in a heap region (decision 0633) whose release walks the free
// lists; a sampled block freed on another thread went onto the region's
// inbox with minus its class still in its header, the release read that
// as the class and wrote list words below the record's lists, and the
// build died with SIGSEGV in heapos.ra_cover on a free list link of 1,
// two runs in nine on darwin/arm64 (one in three in the report). The
// damage was done on every run and the crash was not, so the build runs
// ten times: before the fix at least one of ten failed nine times in
// ten.
func TestHeapProfileRegionBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("ten builds of the compiler")
	}
	t.Parallel()
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	setenv(t, "XO_BACKEND_CACHE", "on")
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(dir, "cache"))
	setenv(t, "XO_BIN_CACHE", "off")
	src := filepath.Join(repoRoot, "bench", "perf", "xo")
	bin := filepath.Join(dir, "a")
	var out, errOut bytes.Buffer
	// the back end's objects first: the profiled builds then spend
	// their time in the front end, where the region is
	if code := run(t, []string{"build", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	setenv(t, "XO_PROF", "heap")
	setenv(t, "XO_PROF_HEAP_RATE", "1")
	setenv(t, "XO_PROF_HEAP_AT", "fe.build")
	for i := 0; i < 10; i++ {
		setenv(t, "XO_PROF_HEAP_OUT", filepath.Join(dir, fmt.Sprintf("fe%d.xoraw", i)))
		// the profile at exit too: its default is xo-prof.xoraw in the
		// working directory, this package's, and a profile never
		// overwrites a file
		setenv(t, "XO_PROF_OUT", filepath.Join(dir, fmt.Sprintf("exit%d.xoraw", i)))
		out.Reset()
		errOut.Reset()
		if code := run(t, []string{"build", "-o", bin, src}, &out, &errOut); code != 0 {
			t.Fatalf("profiled build %d: exit %d: %s", i, code, errOut.String())
		}
	}
}
