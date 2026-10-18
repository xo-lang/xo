package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestArithOracle runs testdata/oracle's property tests, Int arithmetic
// against an exact limb reference (checked_add, checked_sub,
// checked_mul, wrapping_mul, / and %, on random values and Int's edges),
// with `xo test` on the Go backend and every native backend the host
// has. XO_ORACLE_CASES sets the cases per property (default 1000).
func TestArithOracle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a test binary per backend; run without -short")
	}
	cmd, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	cases := 1000
	if n, err := strconv.Atoi(os.Getenv("XO_ORACLE_CASES")); err == nil && n > 0 {
		cases = n
	}
	backends := []string{"go"}
	if _, err := xotool.Clang(); err == nil {
		backends = append(backends, xotool.BackendLLVM)
	}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	dir, err := filepath.Abs(filepath.Join("testdata", "oracle"))
	if err != nil {
		t.Fatal(err)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			args := append(append([]string{}, cmd[1:]...), "test", "--backend="+be, "--cases="+strconv.Itoa(cases), ".")
			c := exec.Command(cmd[0], args...)
			c.Dir = dir
			c.Env = append(os.Environ(), "XO_CRASH_DIR="+t.TempDir())
			out, err := c.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "0 failed") {
				t.Fatalf("xo test --backend=%s: %v\n%s", be, err, out)
			}
		})
	}
}
