package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xo-lang/xo/internal/progen"
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
	for seed := 1; seed <= n; seed++ {
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
			res, err := xotool.Build([]string{dir}, xotool.Options{Env: propEnv(out), Out: filepath.Join(out, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			nres, err := xotool.Build([]string{dir}, xotool.Options{Env: propEnv(out), Backend: native, Out: filepath.Join(out, "prog-native")})
			if err != nil || nres.Failed {
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s", seed, native, err, diagText(nres), numbered(src))
			}
			gr, nr := runProp(t, res.Binaries[xotool.Host()], out), runProp(t, nres.Binaries[xotool.Host()], out)
			if gr != nr {
				t.Errorf("seed %d: the native build runs differently\n--- go\n%s\n--- %s\n%s\n--- program\n%s", seed, gr, native, nr, numbered(src))
			}
		})
	}
}

const (
	deferFaultShort = 8
	deferFaultFull  = 40
)
