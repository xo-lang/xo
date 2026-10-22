package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestFieldGeneratorsInSync runs the Xo generators of the runtime's
// generated field and Keccak code (scripts/gen, ported from Python with
// byte identical output) and checks that each file holds what its
// generator prints now, so an edit by hand or a generator change without
// regenerating fails here. scripts/fieldcheck.py checks the generated
// arithmetic itself.
func TestFieldGeneratorsInSync(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs three programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Skip(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ gen, file string }{
		{"x25519fe64", "std/native/llrt/x25519.xo"},
		{"nistecmul4p", "std/native/llrt/nistec.xo"},
		{"keccaklazy", "std/native/llrt/sha3.xo"},
	} {
		t.Run(c.gen, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			cmd := testtmp.Guard(exec.CommandContext(ctx, xo, "run", filepath.Join("scripts", "gen", c.gen)))
			cmd.Dir = root
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("xo run scripts/gen/%s: %v\n%s", c.gen, err, stderr.String())
			}
			src, err := os.ReadFile(filepath.Join(root, c.file))
			if err != nil {
				t.Fatal(err)
			}
			gen := strings.TrimRight(string(out), "\n")
			if gen == "" || !strings.Contains(string(src), gen) {
				t.Errorf("%s does not hold scripts/gen/%s's output: regenerate it (xo run scripts/gen/%s) and splice it in", c.file, c.gen, c.gen)
			}
		})
	}
}
