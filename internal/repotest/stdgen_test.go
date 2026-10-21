package repotest

import (
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
