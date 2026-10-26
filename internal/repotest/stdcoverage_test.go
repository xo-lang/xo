package repotest

import (
	"os"
	"path/filepath"
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

// TestProgramDirs: ProgramDirs lists the directories a `xo dev edges`
// sweep runs over, so a resolved comparison can be made like with like: the
// corpora, the goldens, the benchmark programs and the scripts, sorted and
// without duplicates, each holding a .xo file.
// `go run ./scripts/stdcoverage -programs` prints it.
func TestProgramDirs(t *testing.T) {
	dirs, err := stdcoverage.ProgramDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) < 800 {
		t.Fatalf("found %d program directories, too few: the list read the wrong tree", len(dirs))
	}
	seen := map[string]bool{}
	for i, d := range dirs {
		if seen[d] {
			t.Errorf("%s is listed twice", d)
		}
		seen[d] = true
		if i > 0 && dirs[i-1] >= d {
			t.Errorf("%s does not follow %s: the list is not sorted", d, dirs[i-1])
		}
		if _, err := os.Stat(filepath.Join(root, d)); err != nil {
			t.Errorf("%s: %v", d, err)
		}
	}
	for _, want := range []string{"bench/perf/aesgcm", "scripts/pgtls", "internal/llvm/testdata/golden/strwhere"} {
		if !seen[want] {
			t.Errorf("%s is not in the program directories", want)
		}
	}
}
