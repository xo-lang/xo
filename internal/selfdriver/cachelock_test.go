//go:build unix

package selfdriver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/gotool"
)

// TestXo1GoBuildSurvivesTrim is internal/build's TestGoCommandSurvivesTrim
// for xo1 (gotchas/go-commands-outside-build-race-trim.md, decision
// 0213): xo1's Go backend runs go build on the Xo Go cache while this
// process trims it the way internal/build's trimGoCache does (xo-gate.lock
// and xo-cache.lock exclusive through gotool.LockCacheExclusive, then
// every entry removed). xo1 must hold the shared lock across go build:
// the builds succeed, and some trims find the cache held.
func TestXo1GoBuildSurvivesTrim(t *testing.T) {
	if testing.Short() {
		t.Skip("builds xo1 and runs go build into an empty cache several times")
	}
	bin, _, err := Binary()
	if err != nil {
		t.Fatal(err)
	}
	std, err := filepath.Abs(filepath.Join("..", "..", "std"))
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	t.Setenv("XO_GOCACHE", cache)
	mod := t.TempDir()
	os.WriteFile(filepath.Join(mod, "xo.mod"), []byte("module example.com/trim\n"), 0o644)
	var refused, trims atomic.Int64
	trim := func() {
		// trimGoCache with a one byte cap once it holds the lock: anything
		// may go
		l := gotool.LockCacheExclusive(false)
		if l == nil {
			refused.Add(1)
			return
		}
		defer l.Unlock()
		trims.Add(1)
		entries, _ := os.ReadDir(cache)
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), "xo-") {
				os.RemoveAll(filepath.Join(cache, e.Name()))
			}
		}
	}
	for round := 0; round < 3; round++ {
		// a new program each round, so xo1's binary cache misses and go
		// build runs
		src := fmt.Sprintf("fn main(os: Os) {\n  os.stdio.println(\"round %d %d\")\n}\n", round, time.Now().UnixNano())
		os.WriteFile(filepath.Join(mod, "main.xo"), []byte(src), 0o644)
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				case <-time.After(20 * time.Millisecond):
					trim()
				}
			}
		}()
		out := filepath.Join(mod, fmt.Sprintf("prog%d", round))
		cmd := exec.Command(bin, "build", "--std="+std, "--wd="+mod, "--backend=go", "-o", out, ".")
		cmd.Dir = mod
		cmd.Env = append(os.Environ(), "PWD="+mod, "XO_GOCACHE="+cache)
		b, err := cmd.CombinedOutput()
		close(stop)
		<-done
		if err != nil {
			t.Fatalf("round %d: xo1 build while trimming: %v\n%s", round, err, b)
		}
		got, err := exec.Command(out).CombinedOutput()
		if err != nil || !strings.HasPrefix(string(got), fmt.Sprintf("round %d ", round)) {
			t.Fatalf("round %d: the program: %v\n%s", round, err, got)
		}
	}
	t.Logf("trims %d, refused while xo1 held the cache %d", trims.Load(), refused.Load())
	if refused.Load() == 0 {
		t.Errorf("no trim found the cache held: xo1 ran go build without the shared lock")
	}
	if trims.Load() == 0 {
		t.Errorf("no trim ran")
	}
}
