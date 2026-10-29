package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdStoreBuilds builds std's store from scratch (xo dev stdcache,
// one variant) in a private cache: the full check of every std body,
// which a whole-program build does lazily. Not skipped under -short, so
// the hourly check sees a std edit that only the store rejects
// (2026-10-29: std/native/http.xo's moved value hid every separately
// compiled build for hours behind a silent whole-program fallback).
func TestStdStoreBuilds(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-stdstore-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+cache)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("std's store does not build: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", out)
	}
	for _, p := range lines {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}
