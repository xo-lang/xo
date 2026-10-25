package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCompare: equal values agree, a different value diverges, and a
// backend with no line for the case diverges too.
func TestCompare(t *testing.T) {
	values := parseOut(map[string][]string{
		"arm64": {"a|1", "b|2"},
		"llvm":  {"a|1", "b|3"},
		"go":    {"a|1"},
	})
	backends := []string{"arm64", "llvm", "go"}
	if _, same := compare(backends, values, "a"); !same {
		t.Error("case a: equal values reported a divergence")
	}
	vals, same := compare(backends, values, "b")
	if same {
		t.Error("case b: 2 against 3 reported no divergence")
	}
	if vals["arm64"] != "2" || vals["llvm"] != "3" {
		t.Errorf("case b: values %v", vals)
	}
	if _, same := compare(backends, values, "c"); same {
		t.Error("case c: a missing line reported no divergence")
	}
}

// TestRunConcurrently: every backend runs, at most three overlap, the
// results come back in backend order, and a failure keeps its message.
func TestRunConcurrently(t *testing.T) {
	backends := []string{"arm64", "llvm", "go", "arm64", "llvm"}
	var mu sync.Mutex
	live, peak := 0, 0
	got := runConcurrently(backends, func(i int, b string) backendRun {
		mu.Lock()
		live++
		if live > peak {
			peak = live
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		live--
		mu.Unlock()
		if i == 3 {
			return backendRun{fail: "backend 3 failed"}
		}
		return backendRun{out: []string{b + "|" + strings.Repeat("0", i)}}
	})
	if peak > 3 {
		t.Fatalf("peak concurrency %d, want at most 3", peak)
	}
	if peak < 2 {
		t.Fatalf("peak concurrency %d, want the backends to overlap", peak)
	}
	for i, b := range backends {
		if i == 3 {
			if got[i].fail != "backend 3 failed" || got[i].out != nil {
				t.Errorf("result %d: %+v, want only its failure", i, got[i])
			}
			continue
		}
		want := b + "|" + strings.Repeat("0", i)
		if len(got[i].out) != 1 || got[i].out[0] != want || got[i].fail != "" {
			t.Errorf("result %d: %+v, want %q", i, got[i], want)
		}
	}
}

// TestReleaseCommands: -release builds the two native backends with
// `xo build --backend=X --release` and leaves the Go backend to `xo run`.
func TestReleaseCommands(t *testing.T) {
	if isReleaseBackend("go") {
		t.Error("the Go backend would be built rather than run")
	}
	for _, b := range []string{"arm64", "llvm"} {
		if !isReleaseBackend(b) {
			t.Errorf("%s: not built for release", b)
		}
		got := strings.Join(buildArgs(b, "/tmp/p-"+b, "/tmp/prog"), " ")
		want := "build --backend=" + b + " --release -o /tmp/p-" + b + " /tmp/prog"
		if got != want {
			t.Errorf("%s: build command %q, want %q", b, got, want)
		}
	}
	if releaseFlag(true) != " -release" || releaseFlag(false) != "" {
		t.Errorf("rerun flag: %q, %q", releaseFlag(true), releaseFlag(false))
	}
	if releaseNote(true) == "" || releaseNote(false) != "" {
		t.Errorf("summary note: %q, %q", releaseNote(true), releaseNote(false))
	}
}
