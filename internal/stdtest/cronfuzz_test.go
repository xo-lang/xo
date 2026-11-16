// A fuzz harness for std/cron (decision 0616's fuzz kind): seeded random
// specs (each field's numbers inside and past its bounds, ranges in and
// out of order, steps including 0 and none, lists, month and day names in
// any field, ?, a field too many or too few, a byte changed, TZ= and
// CRON_TZ= prefixes with good and unknown zones, the descriptors and
// @every with good and bad durations), parsed by ParseStandard or one of
// five NewParser option sets, and a start time from 2019 to 2031; the
// oracle is github.com/robfig/cron/v3 v3.0.1, run by
// scripts/stdcorpus/cronoracle (its own module). Each must parse to
// robfig's next fire time (or none) or fail with robfig's error text. A
// spec robfig itself panics on is left out.
//
// The committed seed 1 corpus is testdata/corpus/cronfuzz (scripts/stdcorpus
// cronfuzz writes it; TestStdCorpora runs it on every backend). TestCronFuzz
// runs a fresh seed (XO_CRONFUZZ_SEED, default 2; XO_CRONFUZZ_N, default
// 2,000) on XO_CRONFUZZ_BACKENDS (default arm64 and llvm); it needs the Go
// module cache to hold robfig/cron (or network access) and is skipped
// without it.
package stdtest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCronFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the cron fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_CRONFUZZ_SEED", 2)
	n := envInt(t, "XO_CRONFUZZ_N", 2000)
	cmd := exec.Command("go", "run", ".", "fuzz", strconv.Itoa(seed), strconv.Itoa(n))
	cmd.Dir = filepath.Join("..", "..", "scripts", "stdcorpus", "cronoracle")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("cronoracle cannot run (robfig/cron not in the module cache?): %v", err)
	}
	runFuzzDir(t, "cronfuzz_test.xo", string(out), envBackends("XO_CRONFUZZ_BACKENDS", []string{"arm64", "llvm"}), n)
}
