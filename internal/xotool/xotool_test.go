package xotool

import (
	"os"
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

// TestInputsSkipTests: a _test.xo file under compiler/ or std/ leaves
// the key unchanged (decision 0262); a source file beside it changes it.
func TestInputsSkipTests(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("compiler/xo.mod", "module xo\n")
	write("compiler/xo/main.xo", "use std/json\nfn main() {}\n")
	write("std/prelude.xo", "")
	write("std/json.xo", "pub fn f() {}\n")
	key := func() string {
		t.Helper()
		k, err := Key(root)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	k0 := key()
	write("compiler/xo/main_test.xo", "test \"t\" { expect 1 == 1 }\n")
	write("std/json_test.xo", "test \"t\" { expect 1 == 1 }\n")
	if k := key(); k != k0 {
		t.Errorf("a _test.xo file changed the key: %s to %s", k0, k)
	}
	write("compiler/xo/more.xo", "fn g() {}\n")
	if k := key(); k == k0 {
		t.Errorf("a new source file left the key at %s", k0)
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

// TestStampArgs: a build of this checkout's compiler/xo is stamped with
// its commit, source key, and seed (decision 0264); a tree without
// compiler/xo/provenance.xo (an older one, a chain hop) is not.
func TestStampArgs(t *testing.T) {
	root := repoRoot(t)
	args := StampArgs(root, "/a/b/seeds/0.20.0/xo")
	got := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] != "--stamp" {
			t.Fatalf("args %q", args)
		}
		name, value, _ := strings.Cut(args[i+1], "=")
		got[name] = value
	}
	key, err := Key(root)
	if err != nil {
		t.Fatal(err)
	}
	if got["XO_SOURCE_KEY"] != key || got["XO_BUILT_BY"] != "seeds/0.20.0/xo" {
		t.Errorf("stamps %v, want key %s", got, key)
	}
	if c := got["XO_COMMIT"]; c != "" && !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(c) {
		t.Errorf("XO_COMMIT %q", c)
	}
	if d := got["XO_DIRTY"]; d != "" && len(d) != 12 {
		t.Errorf("XO_DIRTY %q", d)
	}
	if a := StampArgs(t.TempDir(), "seed"); a != nil {
		t.Errorf("a tree without provenance.xo: %q", a)
	}
}
