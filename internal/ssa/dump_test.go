package ssa_test

import (
	"os"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/ssa"
)

// TestDump prints the SSA of the program in $XO_SSA_DUMP (after InsertRC
// when $XO_SSA_RC is set), or every builder and verifier error. A
// debugging aid: `XO_SSA_DUMP=dir go test ./internal/ssa -run TestDump -v`.
func TestDump(t *testing.T) {
	dir := os.Getenv("XO_SSA_DUMP")
	if dir == "" {
		t.Skip("set XO_SSA_DUMP to a program directory")
	}
	res, ok, err := build.Check([]string{dir}, false)
	if err != nil || !ok {
		if res != nil {
			for _, d := range res.Diagnostics {
				t.Log(diag.Human(d))
			}
		}
		t.Fatalf("check failed: %v", err)
	}
	m, err := ssa.Build(res)
	if err != nil {
		t.Fatalf("build:\n%v", err)
	}
	if err := m.Verify(); err != nil {
		t.Fatalf("verify: %v\n%s", err, m)
	}
	if os.Getenv("XO_SSA_RC") != "" {
		m.InsertRC()
		if err := m.Verify(); err != nil {
			t.Fatalf("verify after InsertRC: %v\n%s", err, m)
		}
	}
	t.Logf("\n%s", m)
}
