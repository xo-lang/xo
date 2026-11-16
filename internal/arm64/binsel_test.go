package arm64

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoldenBinSelectBuild builds every golden program as TestGoldenRun
// does (sharedBuild, the same options, directory and environment) and
// copies each binary to $XO_GOLDEN_BIN_OUT/NAME/prog, for the gate's
// binary selection (decision 0657, scripts/gate.sh bin_select). Built in
// the test's own shape, the head side's builds fill the run cache
// (decision 0360) that TestGoldenRun then hits in the same gate, so the
// selection pays only for the merge base's side; the base side runs this
// test with XO_CLI and XO_STD set to the merge base's xo and std, so both
// sides are built the same way. Skipped without XO_GOLDEN_BIN_OUT.
func TestGoldenBinSelectBuild(t *testing.T) {
	out := os.Getenv("XO_GOLDEN_BIN_OUT")
	if out == "" {
		t.Skip("XO_GOLDEN_BIN_OUT is not set (the gate's binary selection sets it)")
	}
	// an xo that cannot start (a stale XO_CLI without XO_CLI_STALE_OK) is
	// not a golden's build failure: every golden would be "selected" with
	// no binary and the test would pass (2026-11-15)
	if _, err := xotool.Path(); err != nil {
		t.Fatalf("xo: %v", err)
	}
	for _, dir := range goldenDirs(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			o := filepath.Join(out, filepath.Base(dir))
			if err := os.MkdirAll(o, 0o755); err != nil {
				t.Fatal(err)
			}
			res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64})
			if err != nil || res.Failed {
				// a build that fails selects its golden (no binary)
				t.Logf("build: %v %s", err, res.Output)
				return
			}
			data, err := os.ReadFile(res.Binaries[xotool.Host()])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(o, "prog"), data, 0o755); err != nil {
				t.Fatal(err)
			}
		})
	}
}
