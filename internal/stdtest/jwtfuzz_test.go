// A fuzz harness for std/jwt against github.com/golang-jwt/jwt/v5 v5.3.1,
// its reference, for decision 0616's fuzz kind (task #99). The cases come
// from scripts/stdcorpus/jwtoracle's fuzz mode (its own module, so the
// repository's go.mod has no third-party requirement): tokens with random
// headers and claims signed over the exact text, changed as an attacker
// could, verified under random options; std/jwt must give golang-jwt's
// error category for each (the corpus's VERIFY test). TestJwtFuzz runs a
// fresh seed (XO_JWTFUZZ_SEED, default 2; XO_JWTFUZZ_N cases, default
// 2000) on XO_JWTFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestJwtFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the jwt fuzz is a heavy test; its fixed corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_JWTFUZZ_SEED", 2)
	n := envInt(t, "XO_JWTFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "jwtoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("jwtoracle: %v", err)
	}
	runFuzzDir(t, "jwtfuzz_test.xo", string(out), envBackends("XO_JWTFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
