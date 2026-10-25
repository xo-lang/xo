package main

import (
	"strings"
	"testing"
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
