// A fuzz harness for std/idna against golang.org/x/net v0.60.0's idna
// (decision 0616's fuzz kind). The cases come from
// scripts/stdcorpus/idnaoracle's fuzz mode (its own module, so the
// repository's go.mod has no third-party requirement): names of 1 to 4
// labels (sometimes 60 to 140, past 253 bytes) joined by any of the four
// dots, each label text from the corpus pools (sometimes 55 to 70 runes,
// past 63), a random xn-- label, any rune or a run of combining marks; each
// name through ToASCII and ToUnicode on the four profiles, compared on the
// result or the error's code. The fixed seed corpus is testdata/corpus/idna
// (scripts/stdcorpus, TestStdCorpora); TestIdnaFuzz runs a fresh seed
// (XO_IDNAFUZZ_SEED, default 2; XO_IDNAFUZZ_N names, default 2000, 8 checks
// each) on XO_IDNAFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIdnaFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the idna fuzz is a heavy test; its fixed corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_IDNAFUZZ_SEED", 2)
	n := envInt(t, "XO_IDNAFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "idnaoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("idnaoracle: %v", err)
	}
	runFuzzDir(t, "idnafuzz_test.xo", string(out), envBackends("XO_IDNAFUZZ_BACKENDS", []string{"arm64", "llvm"}), 8*n)
}
