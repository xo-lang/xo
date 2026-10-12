package selfhost

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestSelfhostMachoXoTests runs the Xo tests of selfhost/macho, the Mach-O
// assembler and linker of decision 0182 (`xo test`): every instruction
// shape against the system assembler's encoding, logical immediates,
// integer parsing, compact unwind encodings, ULEB128 layout, errors. The
// comparison with `cc -c` on whole programs is scripts/macho-compare.py.
func TestSelfhostMachoXoTests(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds and runs the Xo test binary")
	}
	dir := filepath.Join("..", "..", "selfhost", "macho")
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Test})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("check failed: %+v", res.Diagnostics)
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo test selfhost/macho: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "0 failed") {
		t.Fatalf("unexpected test output:\n%s", out)
	}
}
