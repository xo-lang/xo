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

// TestStackSmallProperty builds programs that make small Bytes and use
// them both in ways decision 0395 keeps in the frame and in ways that
// must keep them on the heap (progen.StackSmallProgram), on the Go
// backend and the native one, and requires both to print the same: a
// frame object read after its frame is gone, or overwritten by the next
// iteration of its site, prints differently.
//
// -short builds stackSmallShort programs; XO_STACK_SMALL_PROPS=N builds N.
func TestStackSmallProperty(t *testing.T) {
	t.Parallel()
	native := ""
	if _, err := xotool.Clang(); err == nil {
		native = xotool.NativeBackend()
	}
	if native == "" {
		t.Skip("no native backend here")
	}
	n := stackSmallFull
	if testing.Short() {
		n = stackSmallShort
	}
	if v := os.Getenv("XO_STACK_SMALL_PROPS"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			t.Fatalf("XO_STACK_SMALL_PROPS=%q: want a positive count", v)
		}
		n = k
	}
	for seed := 1; seed <= n; seed++ {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			src := progen.StackSmallProgram(int64(seed))
			files := map[string]string{"xo.mod": "module example.com/stacksmall\n", "main.xo": src}
			out := t.TempDir()
			dir, err := xotool.TestProgramDir("stacksmall", files)
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
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s", seed, native, err, buildText(nres), numbered(src))
			}
			gr, nr := runProp(t, res.Binaries[xotool.Host()], out), runProp(t, nres.Binaries[xotool.Host()], out)
			if gr != nr {
				t.Errorf("seed %d: the native build runs differently\n--- go\n%s\n--- %s\n%s\n--- program\n%s", seed, gr, native, nr, numbered(src))
			}
		})
	}
}

const (
	stackSmallShort = 4
	stackSmallFull  = 30
)
