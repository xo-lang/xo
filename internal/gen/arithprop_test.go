package gen_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/progen"
)

// arithSeeds is the seeds to run: XO_ARITH_PROP_SEED=S runs only S,
// XO_ARITH_PROPS=N runs 1..N, else 1..def.
func arithSeeds(t *testing.T, def int) []int64 {
	if v := os.Getenv("XO_ARITH_PROP_SEED"); v != "" {
		s, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("XO_ARITH_PROP_SEED=%q", v)
		}
		return []int64{s}
	}
	n := def
	if v := os.Getenv("XO_ARITH_PROPS"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			t.Fatalf("XO_ARITH_PROPS=%q: want a positive count", v)
		}
		n = k
	}
	var out []int64
	for i := 1; i <= n; i++ {
		out = append(out, int64(i))
	}
	return out
}

func writeArith(t *testing.T, seed int64) (string, string) {
	t.Helper()
	src := progen.Arith(seed)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte(fmt.Sprintf("module example.com/arith%d\n", seed)), 0o644)
	os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644)
	return dir, src
}

// TestArithChecks holds progen.Arith to its promise: every program
// checks without a diagnostic. Fast; also under -short.
func TestArithChecks(t *testing.T) {
	for _, seed := range arithSeeds(t, 300) {
		dir, src := writeArith(t, seed)
		res, err := check.Check([]string{dir})
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if len(res.Diagnostics) > 0 {
			t.Fatalf("seed %d: the program does not check:\n%s--- program\n%s", seed, diagText(&build.Result{Diagnostics: res.Diagnostics}), numbered(src))
		}
	}
}

// TestArithProperty is a differential test of the backends on
// progen.Arith programs: each builds with the Go backend, whose generated
// Go must pass go vet, and with every native backend the host has (LLVM
// with clang, arm64 on arm64), and all must exit alike and print the
// same output (runProp). It found an arm64 branch on stale flags
// (testdata/run/flags_same_targets in internal/arm64). XO_ARITH_PROPS=N
// runs seeds 1..N (default 25), XO_ARITH_PROP_SEED=S only S. Skipped
// with -short.
func TestArithProperty(t *testing.T) {
	if testing.Short() {
		t.Skip("builds every program with each backend; run without -short")
	}
	var natives []string
	if _, err := build.Clang(); err == nil {
		natives = append(natives, build.BackendLLVM)
	}
	if runtime.GOARCH == "arm64" {
		natives = append(natives, build.BackendArm64)
	}
	t.Logf("native backends: %v", natives)
	for _, seed := range arithSeeds(t, 25) {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			dir, src := writeArith(t, seed)
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Out: filepath.Join(dir, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			pkgs := []string{"."}
			if _, err := os.Stat(filepath.Join(res.Dir, "m")); err == nil {
				pkgs = append(pkgs, "./m/...")
			}
			if out, err := run(res.Dir, "go", append([]string{"vet"}, pkgs...)...); err != nil {
				t.Errorf("seed %d: go vet: %v\n%s--- program\n%s", seed, err, out, numbered(src))
			}
			want := runProp(t, res.Binaries[build.Host()], dir)
			for _, be := range natives {
				nres, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, NoGC: true, Backend: be, Out: filepath.Join(dir, "prog-"+be)})
				if err != nil || nres.Failed {
					t.Fatalf("seed %d: the %s build failed: %v\n%s\n--- program\n%s", seed, be, err, diagText(nres), numbered(src))
				}
				if got := runProp(t, nres.Binaries[build.Host()], dir); got != want {
					t.Errorf("seed %d: %s runs differently from the Go backend\n--- go\n%s\n--- %s\n%s\n--- program\n%s", seed, be, want, be, got, numbered(src))
				}
			}
		})
	}
}
