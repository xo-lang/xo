package xotool

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTarget: Result.Binaries is keyed by the names xo prints, Go's, so a
// test that names the spec's wasi/wasm32 looks up wasip1/wasm (the
// Linux milestone run of 2026-10-14 failed TestCrossCompile on this).
func TestTarget(t *testing.T) {
	for in, want := range map[string]string{
		"wasi/wasm32":   "wasip1/wasm",
		"wasip1/wasm":   "wasip1/wasm",
		"linux/amd64":   "linux/amd64",
		" darwin/arm64": "darwin/arm64",
		"all":           "all",
	} {
		if got := Target(in); got != want {
			t.Errorf("Target(%q) = %q, want %q", in, got, want)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestInputs: the key covers compiler/ and the std modules it reaches
// (the prelude, std/json through selfhost's `use` lines), not the C
// runtime, which xo reads at run time.
func TestInputs(t *testing.T) {
	root := repoRoot(t)
	in, err := Inputs(root)
	if err != nil {
		t.Fatal(err)
	}
	has := map[string]bool{}
	for _, n := range in {
		has[n] = true
		if strings.HasPrefix(n, "std/llrt/") || strings.HasSuffix(n, ".go") {
			t.Errorf("%s is an input", n)
		}
	}
	for _, n := range []string{"compiler/xo/main.xo", "compiler/xo.mod", "std/prelude.xo", "std/native/prelude.xo"} {
		if !has[n] {
			t.Errorf("%s is not an input", n)
		}
	}
	k1, err := Key(root)
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := Key(root)
	if k1 != k2 || len(k1) != 16 {
		t.Errorf("keys %q %q", k1, k2)
	}
}

// TestChainHops reads scripts/seed-chain.txt: full commit hashes, the std
// of a hop optional.
func TestChainHops(t *testing.T) {
	hops, err := ChainHops(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(hops) == 0 {
		t.Fatal("no hops in scripts/seed-chain.txt")
	}
	hex := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, h := range hops {
		if !hex.MatchString(h.Commit) || h.Std != "" && !hex.MatchString(h.Std) {
			t.Errorf("hop %+v: want full commit hashes", h)
		}
	}
}
