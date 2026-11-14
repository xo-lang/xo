// A fuzz harness for std/norm against golang.org/x/text/unicode/norm
// v0.43.0 (decision 0616's fuzz kind). The cases come from
// scripts/stdcorpus/normoracle's fuzz mode (its own module, so the
// repository's go.mod has no third-party requirement): strings of 0 to 80
// runes mixing combining marks, runes a form changes, Hangul, ASCII and any
// other rune, with runs of up to 40 non-starters (x/text's stream-safe
// U+034F after 30), each checked in the four forms and with
// is_normalized. The fixed seed corpus is testdata/corpus/norm
// (scripts/stdcorpus, TestStdCorpora); TestNormFuzz runs a fresh seed
// (XO_NORMFUZZ_SEED, default 2; XO_NORMFUZZ_N strings, default 2000) on
// XO_NORMFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNormFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the norm fuzz is a heavy test; its fixed corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_NORMFUZZ_SEED", 2)
	n := envInt(t, "XO_NORMFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "normoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("normoracle: %v", err)
	}
	runFuzzDir(t, "normfuzz_test.xo", string(out), envBackends("XO_NORMFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
