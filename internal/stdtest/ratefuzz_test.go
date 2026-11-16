// A fuzz harness for std/rate (decision 0616's fuzz kind): seeded random
// limiters (Inf, 0, Every of a random interval, fractional and whole
// limits; bursts of 0 to 10, a tenth up to 999) driven by up to 40
// operations at a fake clock that moves by 1 ns to 2 s steps and sometimes
// jumps back: tokens, allow_n, reserve_n (ok and delay), a reservation's
// delay later, cancel, set_limit and set_burst. The oracle is
// golang.org/x/time/rate v0.16.0 at the same explicit times, run by
// scripts/stdcorpus/rateoracle (its own module); every answer must be its
// answer, tokens to the bit.
//
// The committed seed 1 corpus is testdata/corpus/ratefuzz (scripts/stdcorpus
// ratefuzz writes it; TestStdCorpora runs it on every backend). TestRateFuzz
// runs a fresh seed (XO_RATEFUZZ_SEED, default 2; XO_RATEFUZZ_N, default
// 2,000) on XO_RATEFUZZ_BACKENDS (default arm64 and llvm); it needs the Go
// module cache to hold x/time (or network access) and is skipped without it.
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRateFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the rate fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_RATEFUZZ_SEED", 2)
	n := envInt(t, "XO_RATEFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "rateoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("rateoracle cannot run (x/time not in the module cache?): %v", err)
	}
	runFuzzDir(t, "ratefuzz_test.xo", string(out), envBackends("XO_RATEFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
