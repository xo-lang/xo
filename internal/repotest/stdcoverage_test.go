package repotest

import (
	"testing"

	"github.com/xo-lang/xo/internal/stdcoverage"
)

// TestStdAPICovered: every top level function and method std/*.xo and
// std/native/prelude.xo declare is called by some corpus or golden, unless
// internal/stdcoverage/allow.txt names it with the reason it cannot be, so
// a new declaration cannot join the API without one or the other. The
// allowlist must also stay honest: a line that matches nothing fails, so an
// entry is removed once a corpus covers what it allowed.
// `go run ./scripts/stdcoverage` prints the same list.
func TestStdAPICovered(t *testing.T) {
	entries, total, err := stdcoverage.Uncovered(root)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1200 {
		t.Fatalf("found %d declarations, too few for std: the scan read the wrong tree", total)
	}
	rules, err := stdcoverage.Allow()
	if err != nil {
		t.Fatal(err)
	}
	used := make([]bool, len(rules))
	for _, e := range entries {
		allowed := false
		for i, r := range rules {
			if r.Match(e) {
				used[i] = true
				allowed = true
			}
		}
		if !allowed {
			t.Errorf("%s.%s (%s) has no corpus or golden call: add one, or a line to internal/stdcoverage/allow.txt with the reason it cannot have one", e.Module, e.Symbol, e.Kind)
		}
	}
	for i, r := range rules {
		if !used[i] {
			t.Errorf("internal/stdcoverage/allow.txt's %q matches no uncovered declaration: remove the line, or correct it", r.Pattern)
		}
	}
}
