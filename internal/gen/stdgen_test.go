package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/stdgen"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestStdGenOutput regenerates std/rt/std_gen.go and std_sigs_gen.go from
// std/*.xo (scripts/stdgen) and fails when a checked-in file differs.
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
	for _, p := range []string{stdgen.Path, stdgen.SigsPath} {
		old, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(old, files[p]) {
			t.Errorf("%s is stale: run go run ./scripts/stdgen", p)
		}
	}
}
