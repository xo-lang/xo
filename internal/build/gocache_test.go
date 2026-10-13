package build

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/gotool"
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
	build := gotool.LockCacheShared()
	trimGoCache()
	build.Unlock()
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
	build := gotool.LockCacheShared()
	done := make(chan struct{})
	go func() { trimGoCache(); close(done) }()
	select {
	case <-done:
		t.Fatal("trim did not wait for the build's lock")
	case <-time.After(200 * time.Millisecond):
	}
	build.Unlock()
	<-done
	checkKept(t, dir, map[string]bool{"a": true, "b": false, "c": false, "d": false})
}

// A go command on Xo's Go cache holds the shared cache lock
// (gotool.HoldCache), so a trim that runs meanwhile (here with a cap of
// one byte, so it removes every entry it may) cannot remove the entries
// the command is reading. Each round warms the cache, edits a slow
// package so go vet finds fmt and strings in the cache but must compile
// and vet the slow package first, and trims during that compile. Without
// the lock go vet fails with `could not import strings (open .../-d: no
// such file or directory)`, as in the 2026-10-13 batch 6 full run
// (gotchas/go-commands-outside-build-race-trim.md).
func TestGoCommandSurvivesTrim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no cache lock without flock")
	}
	if testing.Short() {
		t.Skip("compiles fmt into an empty cache several times")
	}
	dir := t.TempDir()
	t.Setenv("XO_GOCACHE", dir)
	t.Setenv("XO_GOCACHE_MAX", "1")
	mod := t.TempDir()
	os.MkdirAll(filepath.Join(mod, "slow"), 0o755)
	os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module m\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(mod, "m.go"), []byte("package main\n\nimport (\n\t\"fmt\"\n\t\"m/slow\"\n\t\"strings\"\n)\n\nfunc main() { fmt.Println(strings.ToUpper(\"x\"), slow.F0(1)) }\n"), 0o644)
	writeSlow := func(round int) {
		var b strings.Builder
		fmt.Fprintf(&b, "package slow\n\nconst round = %d\n", round)
		for i := 0; i < 20000; i++ {
			fmt.Fprintf(&b, "\nfunc F%d(x int) int {\n\tif x > %d {\n\t\treturn x*%d + round\n\t}\n\treturn x - %d\n}\n", i, i, i, i)
		}
		os.WriteFile(filepath.Join(mod, "slow", "slow.go"), []byte(b.String()), 0o644)
	}
	vet := func(hold bool) ([]byte, error) {
		cmd := exec.Command(goTool(), "vet", "./...")
		cmd.Dir = mod
		cmd.Env = GoEnv()
		release := func() {}
		if hold {
			release = gotool.HoldCache()
		}
		defer release()
		return cmd.CombinedOutput()
	}
	for round, delay := range []time.Duration{100, 300, 600, 900} {
		writeSlow(round)
		if out, err := vet(false); err != nil { // warm the cache, no trim
			t.Fatalf("warm go vet: %v\n%s", err, out)
		}
		writeSlow(round + 100)
		done := make(chan struct{})
		go func() {
			time.Sleep(delay * time.Millisecond)
			os.Remove(filepath.Join(dir, "xo-trim-stamp"))
			trimGoCache()
			close(done)
		}()
		start := time.Now()
		out, err := vet(true)
		t.Logf("vet %v, delay %v", time.Since(start), delay*time.Millisecond)
		<-done
		if err != nil {
			t.Fatalf("go vet with a trim after %v: %v\n%s", delay*time.Millisecond, err, out)
		}
	}
}
