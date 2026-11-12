package repotest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/stdcoverage"
)

// TestStdAPICovered: every top level function and method std/*.xo and
// std/native/prelude.xo declare is reached by a resolved call from some
// corpus, golden, benchmark or script program, unless
// internal/stdcoverage/allow.txt names it with the reason it cannot be, so
// a new declaration cannot join the API without one or the other. A
// declaration counts only when a call resolves to it, directly or through
// std's bodies (`xo dev edges`), not when some call has its name. The
// allowlist must also stay honest: a line that matches nothing fails, so an
// entry is removed once a program covers what it allowed.
//
// The sweep checks every program (stdcoverage.Sweep, XO_STDCOV_JOBS at a
// time, default 2; about an hour in tier 69 with NumCPU jobs, its time at 2
// measured in the next tier), so it runs where XO_STDCOV_SWEEP=1 and XO_CLI name an xo:
// gate.sh's stdcov step, run when named and by the 8-hourly tier.
// `go run ./scripts/stdcoverage -sweep` prints the same list.
func TestStdAPICovered(t *testing.T) {
	if os.Getenv("XO_STDCOV_SWEEP") != "1" {
		t.Skip("the resolved sweep runs in gate.sh's stdcov step (XO_STDCOV_SWEEP=1 with XO_CLI)")
	}
	xo := os.Getenv("XO_CLI")
	if xo == "" {
		t.Fatal("XO_STDCOV_SWEEP=1 needs XO_CLI, the xo to sweep with")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	edges, failed, err := stdcoverage.Sweep(abs, xo, t.TempDir(), stdcoverage.Jobs())
	if err != nil {
		t.Fatal(err)
	}
	// a program the sweep could not check reaches nothing, so what only it
	// calls would read as uncovered
	for _, f := range failed {
		t.Errorf("xo dev edges failed, so its calls are not counted: %s", f)
	}
	entries, total, _, err := stdcoverage.UncoveredResolved(abs, edges)
	if err != nil {
		t.Fatal(err)
	}
	if total < 1200 {
		t.Fatalf("found %d declarations, too few for std: the sweep read the wrong tree", total)
	}
	rules, err := stdcoverage.Allow()
	if err != nil {
		t.Fatal(err)
	}
	open, unused := stdcoverage.Unallowed(entries, rules)
	for _, e := range open {
		t.Errorf("%s.%s (%s) has no resolved call from a corpus or golden: add one, or a line to internal/stdcoverage/allow.txt with the reason it cannot have one", e.Module, e.Symbol, e.Kind)
	}
	for _, r := range unused {
		t.Errorf("internal/stdcoverage/allow.txt's %q matches no uncovered declaration: remove the line, or correct it", r.Pattern)
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
		// a .xo directory holds the bindings xo writes for a program
		// (.xo/gobind), which are not programs of their own
		if strings.Contains("/"+filepath.ToSlash(d)+"/", "/.xo/") {
			t.Errorf("%s is inside a .xo directory", d)
		}
	}
	for _, want := range []string{"bench/perf/aesgcm", "scripts/pgtls", "internal/llvm/testdata/golden/strwhere"} {
		if !seen[want] {
			t.Errorf("%s is not in the program directories", want)
		}
	}
}
