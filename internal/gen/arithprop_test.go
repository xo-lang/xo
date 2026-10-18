package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
	"github.com/xo-lang/xo/internal/xotool"
)

// arithSeeds is the seeds to run: XO_ARITH_PROP_SEED=S runs only S,
// XO_ARITH_PROPS=N runs N seeds, else def, from XO_ARITH_PROP_FIRST
// (default 1; bench/nightly.sh moves it every day).
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
	first := int64(1)
	if v := os.Getenv("XO_ARITH_PROP_FIRST"); v != "" {
		k, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("XO_ARITH_PROP_FIRST=%q", v)
		}
		first = k
	}
	var out []int64
	for i := 0; i < n; i++ {
		out = append(out, first+int64(i))
	}
	return out
}

// arithFiles is the module of progen.Arith program seed (writeArith).
func arithFiles(seed int64, src string) map[string]string {
	return map[string]string{"xo.mod": fmt.Sprintf("module example.com/arith%d\n", seed), "main.xo": src}
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
	t.Parallel() // temporary directories only
	// one xo check of 25 programs (each its own module) at a time, in
	// parallel; a batch with a diagnostic is checked again one program at
	// a time to name the seed (one check per program in turn took about
	// 4 s)
	seeds := arithSeeds(t, 300)
	type prog struct {
		seed     int64
		dir, src string
	}
	var batches [][]prog
	for i, seed := range seeds {
		dir, src := writeArith(t, seed)
		if i%25 == 0 {
			batches = append(batches, nil)
		}
		batches[len(batches)-1] = append(batches[len(batches)-1], prog{seed, dir, src})
	}
	checkOne := func(p prog) {
		ds, err := xotool.Check([]string{p.dir})
		if err != nil {
			t.Errorf("seed %d: %v", p.seed, err)
		} else if len(ds) > 0 {
			t.Errorf("seed %d: the program does not check:\n%s--- program\n%s", p.seed, diagText(&xotool.Result{Diagnostics: ds}), numbered(p.src))
		}
	}
	var wg sync.WaitGroup
	for _, b := range batches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var dirs []string
			for _, p := range b {
				dirs = append(dirs, p.dir)
			}
			if ds, err := xotool.Check(dirs); err == nil && len(ds) == 0 {
				return
			}
			for _, p := range b {
				checkOne(p)
			}
		}()
	}
	wg.Wait()
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
	if _, err := xotool.Clang(); err == nil {
		natives = append(natives, xotool.BackendLLVM)
	}
	if runtime.GOARCH == "arm64" {
		natives = append(natives, xotool.BackendArm64)
	}
	t.Logf("native backends: %v", natives)
	for _, seed := range arithSeeds(t, 25) {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			dir, src := writeArith(t, seed)
			tree := filepath.Join(dir, "gotree")
			res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(dir, "prog"), Flags: []string{"--go-tree=" + tree}})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			pkgs := []string{"."}
			if _, err := os.Stat(filepath.Join(tree, "m")); err == nil {
				pkgs = append(pkgs, "./m/...")
			}
			// -bools=false: progen.Arith can write `a > b || a > b`, which
			// the Go backend keeps as written (seeds 1315 and 1546)
			if out, err := run(tree, "go", append([]string{"vet", "-bools=false"}, pkgs...)...); err != nil {
				t.Errorf("seed %d: go vet: %v\n%s--- program\n%s", seed, err, out, numbered(src))
			}
			want := runProp(t, res.Binaries[xotool.Host()], dir)
			for _, be := range natives {
				nres, err := xotool.Build([]string{dir}, xotool.Options{NoGC: true, Backend: be, Out: filepath.Join(dir, "prog-"+be)})
				if err != nil || nres.Failed {
					t.Fatalf("seed %d: the %s build failed: %v\n%s\n--- program\n%s%s", seed, be, err, buildText(nres), numbered(src), triage("arith", seed, arithFiles(seed, src), be, true))
				}
				if got := runProp(t, nres.Binaries[xotool.Host()], dir); got != want {
					t.Errorf("seed %d: %s runs differently from the Go backend\n--- go\n%s\n--- %s\n%s\n--- program\n%s%s", seed, be, want, be, got, numbered(src), triage("arith", seed, arithFiles(seed, src), be, false))
				}
			}
		})
	}
}
