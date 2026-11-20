package llvm

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoldenBinSelectBuild builds every golden program as TestGolden does
// (the same options, goldenEnv's optimization level and LTO, the same
// directory) and copies each binary to $XO_GOLDEN_BIN_OUT/NAME/prog, for
// the gate's binary selection of the llvm step (decision 0657 stage 2,
// scripts/gate.sh bin_select llvm). Built in TestGolden's shape, the head
// side's builds fill the run cache (decision 0360) that TestGolden then
// hits in the same gate, so the selection pays only for the merge base's
// side; the base side runs this test with XO_CLI and XO_STD set to the
// merge base's xo and std. XO_LLVM_GOLDENS limits it to those goldens (a
// check of the test itself); the gate leaves it unset. Skipped without
// XO_GOLDEN_BIN_OUT or clang.
func TestGoldenBinSelectBuild(t *testing.T) {
	out := os.Getenv("XO_GOLDEN_BIN_OUT")
	if out == "" {
		t.Skip("XO_GOLDEN_BIN_OUT is not set (the gate's binary selection sets it)")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	// an xo that cannot start is not a golden's build failure: every
	// golden would be "selected" with no binary and the test would pass
	// (the arm64 test's lesson, 2026-11-15)
	if _, err := xotool.Path(); err != nil {
		t.Fatalf("xo: %v", err)
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	// a build environment that fails every golden (no std, a broken
	// clang) is not a selection of every golden: the side fails, so the
	// gate's note says so (2026-11-19: XO_STD unset built nothing and the
	// test passed)
	var tried, built atomic.Int64
	t.Run("all", func(t *testing.T) { buildGoldenBins(t, dirs, out, &tried, &built) })
	if tried.Load() > 0 && built.Load() == 0 {
		t.Fatalf("none of %d goldens built: the build environment, not the change", tried.Load())
	}
}

func buildGoldenBins(t *testing.T, dirs []string, out string, tried, built *atomic.Int64) {
	for _, dir := range dirs {
		dir := dir
		name := filepath.Base(dir)
		if !goldenSelected(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := filepath.Join(out, name)
			if err := os.MkdirAll(o, 0o755); err != nil {
				t.Fatal(err)
			}
			tried.Add(1)
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Env: goldenEnv(dir)})
			// a build that fails selects its golden (no binary)
			if err != nil {
				t.Logf("build: %v", err)
				return
			}
			if res.Failed {
				t.Logf("build failed: %s", res.Output)
				return
			}
			data, err := os.ReadFile(res.Binaries[xotool.Host()])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(o, "prog"), data, 0o755); err != nil {
				t.Fatal(err)
			}
			built.Add(1)
		})
	}
}
