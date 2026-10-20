package repotest

import (
	"testing"

	"github.com/xo-lang/xo/internal/stdgen"
)

// TestStdGenCurrent fails when std/*.xo's type declarations changed
// since std/rt/std_gen.go was generated (the hash its third line
// records), without running xo; internal/gen's TestStdGenOutput checks
// the whole file.
func TestStdGenCurrent(t *testing.T) {
	want, err := stdgen.Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := stdgen.Recorded(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s was generated from other std type declarations (recorded %q, std/*.xo now %q): run go run ./scripts/stdgen", stdgen.Path, got, want)
	}
}
