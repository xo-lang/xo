package arm64

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestRcpartFlat runs bench/perf/rcpart's rows: each is a way a Str, List or
// Map leaf of a borrowed struct parameter stops updating in place, about a
// second a row against a few milliseconds, so a change to borrow inference
// that loses one shows here (bench/results/part-borrow-2026-11-02.md).
func TestRcpartFlat(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	res, err := sharedBuild("../../bench/perf/rcpart", xotool.Options{Backend: xotool.BackendArm64})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	out, err := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()], "check")).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(string(out), "slow ") || !strings.Contains(string(out), " 0 slow") {
		t.Fatalf("a row is quadratic:\n%s", out)
	}
}
