// A fuzz harness for std/bcrypt against golang.org/x/crypto v0.58.0's
// bcrypt, its reference, for decision 0616's fuzz kind (task #99). The
// cases come from scripts/stdcorpus/bcryptoracle's fuzz mode (its own
// module, so the repository's go.mod has no third-party requirement):
// hashes Go made, changed as a stored or attacker-supplied hash could be,
// compared with right, wrong and over-long passwords and read by Cost;
// std/bcrypt must answer as Go, and refuse where Go panics. TestBcryptFuzz
// runs a fresh seed (XO_BCRYPTFUZZ_SEED, default 2; XO_BCRYPTFUZZ_N cases,
// default 2000) on XO_BCRYPTFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBcryptFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the bcrypt fuzz is a heavy test; its fixed corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_BCRYPTFUZZ_SEED", 2)
	n := envInt(t, "XO_BCRYPTFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "bcryptoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bcryptoracle: %v", err)
	}
	runFuzzDir(t, "bcryptfuzz_test.xo", string(out), envBackends("XO_BCRYPTFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
