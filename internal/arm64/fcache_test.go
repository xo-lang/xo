package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestBackendCacheModuleFacts shares one per-function back-end cache
// (decision 0220 addendum stage B) between a C library build and a no C
// library build of one program (linux/arm64 assembly, either first):
// each build's text equals its generation with the cache off, and
// XO_BACKEND_CACHE=verify passes. A function's cached text must not
// depend on facts of the module it was generated in (2026-10-17: weak_ref
// wrote `.weak __errno_location` only when the module did not define it,
// so a libc build's entries gave a no-libc build `.weak` lines; and
// c_sym_addr's direct or GOT address depends on nolibc, now in the key).
func TestBackendCacheModuleFacts(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(filepath.Join("..", "llvm", "testdata", "golden", "arith"))
	if err != nil {
		t.Fatal(err)
	}
	asm := func(t *testing.T, cache string, env ...string) (string, error) {
		cmd := exec.Command(xo, "asm", "--backend=arm64", "--targets=linux/arm64", ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		out, err := cmd.Output()
		return string(out), err
	}
	for _, order := range [][2]string{{"0", "1"}, {"1", "0"}} {
		t.Run("nolibc"+order[0]+"_then_"+order[1], func(t *testing.T) {
			cache, err := testtmp.Dir("xo-fcache-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(cache)
			for _, nl := range order {
				want, err := asm(t, cache, "XO_NOLIBC="+nl, "XO_BACKEND_CACHE=off")
				if err != nil {
					t.Fatalf("XO_NOLIBC=%s, cache off: %v", nl, err)
				}
				got, err := asm(t, cache, "XO_NOLIBC="+nl)
				if err != nil {
					t.Fatalf("XO_NOLIBC=%s: %v", nl, err)
				}
				if got != want {
					t.Errorf("XO_NOLIBC=%s: the cached text differs from the generation with the cache off", nl)
				}
				if _, err := asm(t, cache, "XO_NOLIBC="+nl, "XO_BACKEND_CACHE=verify"); err != nil {
					t.Errorf("XO_NOLIBC=%s: XO_BACKEND_CACHE=verify: %v", nl, err)
				}
			}
		})
	}
}
