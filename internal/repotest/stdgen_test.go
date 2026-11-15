package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/stdgen"
)

// TestStdGenCurrent fails when a std module's type declarations changed
// since its std/rt/std_gen_M.go was generated (the hash its third line
// records), without running xo; internal/gen's TestStdGenOutput checks
// the whole files.
func TestStdGenCurrent(t *testing.T) {
	want, err := stdgen.Hashes(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := stdgen.Recorded(root)
	if err != nil {
		t.Fatal(err)
	}
	for m, h := range got {
		if w, ok := want[m]; !ok {
			t.Errorf("%s: no std module %s: run go run ./scripts/stdgen", stdgen.TypesPath(m), m)
		} else if h != w {
			t.Errorf("%s was generated from other std type declarations (recorded %q, std now %q): run go run ./scripts/stdgen", stdgen.TypesPath(m), h, w)
		}
	}
}

// tempName matches a temporary the Go backend names (compiler/gen tmp):
// x_ and a lowercase prefix, then its number.
var tempName = regexp.MustCompile(`\bx_[a-z]+([0-9]+)\b`)

// TestStdGenTempsPerModule checks that each std module's generated Go
// numbers its temporaries from its own count (compiler/gen Pkg.tmps):
// std/rt is one Go package, and one count across every module made an
// edit to one module renumber the temporaries of every later one (owner,
// 2026-11-15, from the agents' survey: a std/png change rewrote
// std_gen_prelude.go, regexp, task, textproto, trace and url). A module's
// lowest temporary is then small; with one count it was in the
// thousands (png's first was x_t1464).
func TestStdGenTempsPerModule(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "std", "rt", "std_gen_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no std/rt/std_gen_*.go: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		low := -1
		for _, m := range tempName.FindAllSubmatch(b, -1) {
			n, _ := strconv.Atoi(string(m[1]))
			if low < 0 || n < low {
				low = n
			}
		}
		if low > 100 {
			t.Errorf("%s: its lowest temporary is x_..%d: temporaries are numbered across modules again (compiler/gen tmp)", filepath.Base(f), low)
		}
	}
}
