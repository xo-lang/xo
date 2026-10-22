package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestDeferFaultProperty builds programs whose functions fault inside
// deferred code (progen.DeferFaultProgram: deferred blocks nested in
// deferred blocks, `let` and `for` patterns whose value faults, deferred
// code in loops, spawned tasks) on the Go backend and the native one, and
// requires both to print the same: the deferred code registered before a
// fault runs, then the blocks around it (core 5.6, decision 0085). Before
// 2026-10-17 the native backends skipped it in 14 of 20 such programs
// (gotchas/native-deferred-block-fault-skips-defers.md).
//
// -short builds deferFaultShort programs; XO_DEFER_FAULT_PROPS=N builds N.
func TestDeferFaultProperty(t *testing.T) {
	t.Parallel()
	native := ""
	if _, err := xotool.Clang(); err == nil {
		native = xotool.NativeBackend()
	}
	if native == "" {
		t.Skip("no native backend here")
	}
	n := deferFaultFull
	if testing.Short() {
		n = deferFaultShort
	}
	if v := os.Getenv("XO_DEFER_FAULT_PROPS"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			t.Fatalf("XO_DEFER_FAULT_PROPS=%q: want a positive count", v)
		}
		n = k
	}
	// XO_DEFER_FAULT_PROP_FIRST: the first seed, for batches
	// (scripts/progen-batches.sh)
	first := 1
	if v, err := strconv.Atoi(os.Getenv("XO_DEFER_FAULT_PROP_FIRST")); err == nil && v > 0 {
		first = v
	}
	for seed := first; seed < first+n; seed++ {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			src := progen.DeferFaultProgram(int64(seed))
			files := map[string]string{"xo.mod": "module example.com/deferfault\n", "main.xo": src}
			out := t.TempDir()
			dir, err := xotool.TestProgramDir("deferfault", files)
			if err != nil {
				dir = out
				for name, text := range files {
					os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
				}
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(out, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			nres, err := xotool.Build([]string{dir}, xotool.Options{Backend: native, Out: filepath.Join(out, "prog-native")})
			if err != nil || nres.Failed {
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s%s", seed, native, err, buildText(nres), numbered(src), triage("deferfault", int64(seed), files, native, true))
			}
			gr, nr := runProp(t, res.Binaries[xotool.Host()], out), runProp(t, nres.Binaries[xotool.Host()], out)
			if gr != nr {
				t.Errorf("seed %d: the native build runs differently\n--- go\n%s\n--- %s\n%s\n--- program\n%s%s", seed, gr, native, nr, numbered(src), triage("deferfault", int64(seed), files, native, false))
			}
		})
	}
}

// TestDeferFaultBlocks bounds the blocks the SSA builder makes for
// deferfault seed 8's case2 (deferred code nested three deep, faults in
// it and around it): a copy of the deferred code at every landing made
// 225,753 blocks, a 30 GB arm64 build that outlived its test's timeout
// (2026-10-21); the shared unwind blocks (compiler/ssa unwind_block,
// defer_fault_block) make 4,288.
func TestDeferFaultBlocks(t *testing.T) {
	t.Parallel()
	cli, err := xotool.Command()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	for name, text := range map[string]string{"xo.mod": "module example.com/deferfault\n", "main.xo": progen.DeferFaultProgram(8)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	args := append(append([]string{}, cli[1:]...), "dev", "dump", "ssa", "--fn=case2", "--stage=build", ".")
	cmd := testtmp.Guard(exec.Command(cli[0], args...))
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo dev dump ssa: %v\n%s", err, out)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) > 1 && line[0] == 'b' && line[1] >= '0' && line[1] <= '9' {
			n++
		}
	}
	if n == 0 || n > 8000 {
		t.Fatalf("case2 has %d blocks after the SSA build, want 1 to 8000", n)
	}
}

const (
	deferFaultShort = 8
	deferFaultFull  = 40
)
