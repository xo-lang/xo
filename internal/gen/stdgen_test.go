package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/stdgen"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdGenOutput regenerates std/rt/std_gen_M.go and std_sigs_M.go
// from std/*.xo (scripts/stdgen) and fails when a checked-in file
// differs, is missing, or is left over.
func TestStdGenOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("runs xo; internal/repotest's TestStdGenCurrent checks the declarations' hash")
	}
	cmd, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	files, err := stdgen.Generate(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	have, err := stdgen.Generated(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range have {
		if _, ok := files[p]; !ok {
			t.Errorf("%s is left over: run go run ./scripts/stdgen", p)
		}
	}
	for p, want := range files {
		old, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Errorf("%s is missing: run go run ./scripts/stdgen", p)
			continue
		}
		if !bytes.Equal(old, want) {
			t.Errorf("%s is stale: run go run ./scripts/stdgen", p)
		}
	}
}
