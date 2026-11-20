package llvm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestGoldenBinSelectSharesCache: a golden built as the gate's binary
// selection builds it (XO_GOLDEN_BIN_OUT set, XO_LLVM_GOLDENS unset) and
// then as TestGolden builds it in the same gate (XO_LLVM_GOLDENS naming
// the selection) is one entry of xo's binary cache: the selection's head
// side is then paid once (decision 0657 stage 2, lever b). xo hashes every
// XO_ variable into the run cache key, so the harness's own variables made
// every TestGolden build miss (2026-11-19: 54 entries after the
// selection, 105 after TestGolden).
func TestGoldenBinSelectSharesCache(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	cache := t.TempDir()
	t.Setenv("XO_BIN_CACHE", cache)
	dir := filepath.Join("testdata", "golden", "abs_fault")
	build := func() {
		t.Helper()
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: xotool.BackendLLVM, Env: goldenEnv(dir)})
		if err != nil || res.Failed {
			t.Fatalf("build: %v %v", err, res)
		}
	}
	t.Setenv("XO_GOLDEN_BIN_OUT", t.TempDir())
	t.Setenv("XO_LLVM_GOLDENS", "")
	os.Unsetenv("XO_LLVM_GOLDENS")
	build()
	n1 := countFiles(t, cache)
	if n1 == 0 {
		t.Fatalf("the selection's build stored nothing in %s", cache)
	}
	os.Unsetenv("XO_GOLDEN_BIN_OUT") // t.Setenv above restores it
	t.Setenv("XO_LLVM_GOLDENS", "abs_fault")
	build()
	if n2 := countFiles(t, cache); n2 != n1 {
		t.Fatalf("TestGolden's build missed the selection's entry: %d files in the cache after the selection, %d after", n1, n2)
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
