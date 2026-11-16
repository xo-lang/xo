// A fresh seed fuzz run for std/big against Go's math/big (task #116,
// decision 0616's fuzz kind). The cases are the corpus generator's
// (scripts/stdcorpus/big.go: seeded random operands of up to 1,500 bits,
// runs of ones and zeros, both signs, primes, operands past Karatsuba's
// threshold and of very different sizes, every std/big operation, SetString
// in each base, and Jacobi), made with XO_BIGFUZZ_SEED added to each of its
// seeds and XO_BIGFUZZ_N operand pairs (default 2000); seed 0 is the
// committed testdata/corpus/big, which TestStdCorpora runs. Xo must give
// Go's values, texts and errors. TestBigFuzz runs XO_BIGFUZZ_SEED (default
// 2) on XO_BIGFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestBigFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the big fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_BIGFUZZ_SEED", 2)
	n := envInt(t, "XO_BIGFUZZ_N", 2000)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "big")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_BIGFUZZ_SEED="+strconv.Itoa(seed), "XO_BIGFUZZ_N="+strconv.Itoa(n), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scripts/stdcorpus big: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "big_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "big_test.xo", string(text), envBackends("XO_BIGFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
