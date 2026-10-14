package benchharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestHarness runs bench/harness/test_harness.py. It needs python3 and the
// go command on PATH, and never calls an external endpoint (the adapter
// test uses a fake server on 127.0.0.1).
//
// The xo commands it runs reuse built binaries (the binary cache) and
// their -race test binaries do not sleep 1 s at exit (GORACE
// atexit_sleep_ms=0, gotchas/race-binaries-sleep-at-exit.md); together
// 24 s became 7 s on 2026-10-09. Each test method runs in its own
// python3 process (-k NAME), in parallel: in one process they took
// about 12 to 19 s under load, the two adapter tests 6 to 7 s each
// (bench/results/short-tests-2026-10-14.md).
func TestHarness(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "bench", "harness", "test_harness.py"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	// every test method: each name is a -k pattern that selects it (and
	// any method whose name contains it), so together they run them all
	names := regexp.MustCompile(`(?m)^    def (test_\w+)\(self`).FindAllSubmatch(src, -1)
	if len(names) == 0 {
		t.Fatalf("no test methods in %s", script)
	}
	xotool.EnableTestBinCache()
	env := os.Environ()
	// the xo the harness runs, built here (internal/xotool) and passed as
	// $XO_BIN: otherwise the first adapter test builds it through `go run
	// ./scripts/xobuild` inside its 600 s subprocess timeout, which a
	// cold build on linux/amd64 exceeded (gotchas/harness-timeout-includes-xo-build.md)
	if _, ok := os.LookupEnv("XO_BIN"); !ok {
		xo, err := xotool.Path()
		if err != nil {
			t.Fatalf("no xo for the harness: %v", err)
		}
		env = append(env, "XO_BIN="+xo)
	}
	if _, ok := os.LookupEnv("GORACE"); !ok {
		env = append(env, "GORACE=atexit_sleep_ms=0")
	}
	for _, m := range names {
		name := string(m[1])
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(py, "-I", script, "-k", name)
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("harness self test %s failed: %v\n%s", name, err, out)
			}
		})
	}
}
