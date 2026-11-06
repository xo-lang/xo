// A fresh seed fuzz run for std/timego against Go's time package (decision 0461: a parser of untrusted input is
// Early on testing until it is fuzzed). The cases are the corpus generator's (scripts/stdcorpus/timego.go: Go
// layouts formatted at random instants and offsets, Go's output parsed back, and one in three parse inputs
// changed: a character replaced, deleted or inserted, or the text cut), made with XO_TIMEGOFUZZ_SEED added to its
// seed; seed 0 is the committed testdata/corpus/timego, which TestStdCorpora runs on every corpus backend.
// TestTimegoFuzz runs XO_TIMEGOFUZZ_SEED (default 2) on XO_TIMEGOFUZZ_BACKENDS (default arm64 and llvm); it skips
// when the oracle cannot run.
package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

func TestTimegoFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the timego fuzz is a heavy test; its seed 0 corpus runs in TestStdCorpora")
	}
	seed := envInt(t, "XO_TIMEGOFUZZ_SEED", 2)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "./scripts/stdcorpus", "timego")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_TIMEGOFUZZ_SEED="+strconv.Itoa(seed), "XO_STDCORPUS_OUT="+out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("the oracle (internal/stdtest/timeref) cannot run here: %v\n%s", err, b)
	}
	text, err := os.ReadFile(filepath.Join(out, "timego_test.xo"))
	if err != nil {
		t.Fatal(err)
	}
	runFuzzDir(t, "timego_test.xo", string(text), envBackends("XO_TIMEGOFUZZ_BACKENDS", []string{"arm64", "llvm"}), 0)
}
