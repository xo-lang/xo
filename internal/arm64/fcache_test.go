package arm64

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
				got, err := asm(t, cache, "XO_NOLIBC="+nl, "XO_BACKEND_CACHE=on")
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

// TestBackendCacheEditIdentity is the edit loop's invariant (decision
// 0220 addendum stage D): a build through a warm back-end cache is the
// same file as a full build of the same source. One program of 300
// functions (enough for the per-function caches) and one cache directory:
// a first build, a second, an edit of one function's constant, a
// statement added to it, and the revert; after each, the cached build's
// binary must equal the binary of XO_BACKEND_CACHE=off, for the host and
// for linux/arm64 (an ELF with DWARF: a decoded function that lost its
// inline sites changed only that, 2026-10-18). XO_BACKEND_CACHE=on is
// set, since a gate may export off (scripts/gate.sh).
func TestBackendCacheEditIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-editid-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	gen := func(k int, stmt bool) string {
		var b strings.Builder
		for i := 0; i < 300; i++ {
			fmt.Fprintf(&b, "fn f%d(x: Int) -> Int {\n", i)
			if i == 150 && stmt {
				b.WriteString("  if x == 12345 {\n    return 1\n  }\n")
			}
			mul := 3
			if i == 150 {
				mul = k
			}
			if i == 299 {
				fmt.Fprintf(&b, "  x * %d\n}\n\n", mul)
				continue
			}
			fmt.Fprintf(&b, "  if x > %d {\n    f%d(x - 1) + %d\n  } else {\n    x * %d\n  }\n}\n\n", i, i+1, i, mul)
		}
		// Float constants: the reference counting entries keep them by
		// their bits (rc cache v8; before, such a function was never cached)
		b.WriteString("fn scale(x: Int) -> Float {\n  Float(x) * 0.1 + 2.5e-300 - 1.5\n}\n\n")
		b.WriteString("fn main(os: Os) {\n  os.stdio.println(\"${f0(400)} ${scale(7)}\")\n}\n")
		return b.String()
	}
	build := func(t *testing.T, name, target string, env ...string) ([]byte, string) {
		// the same file name in both directories: a Mach-O signature names it
		out := filepath.Join(work, name, strings.ReplaceAll(target, "/", "-"), "prog")
		args := []string{"build", "--backend=arm64", "--phase-times", "-o", out}
		if target != "" {
			args = append(args, "--targets="+target)
		}
		cmd := exec.Command(xo, append(args, ".")...)
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	// "cache functions 312/320 reused": some read from the cache
	reused := func(log, what string) bool {
		m := regexp.MustCompile(`cache ` + what + ` (\d+)/\d+ reused`).FindStringSubmatch(log)
		return m != nil && m[1] != "0"
	}
	for _, s := range []struct {
		name string
		k    int
		stmt bool
	}{{"first", 3, false}, {"again", 3, false}, {"edit", 5, false}, {"statement", 5, true}, {"revert", 3, false}} {
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(gen(s.k, s.stmt)), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, target := range []string{"", "linux/arm64"} {
			on, log := build(t, "on", target, "XO_BACKEND_CACHE=on")
			off, _ := build(t, "off", target, "XO_BACKEND_CACHE=off")
			if !bytes.Equal(on, off) {
				t.Errorf("%s %s: the build through the cache differs from the build with XO_BACKEND_CACHE=off", s.name, target)
			}
			// the front end cache (decision 0220 addendum stage 2): bodies
			// replayed from the second build on
			if s.name != "first" && !regexp.MustCompile(`cache fe [1-9]\d*/\d+ bodies`).MatchString(log) {
				t.Errorf("%s %s: no function body read from the front end cache:\n%s", s.name, target, log)
			}
			if s.name != "first" && (!reused(log, "rc") || !reused(log, "functions")) {
				t.Errorf("%s %s: no function read from the cache, so the test compares nothing:\n%s", s.name, target, log)
			}
			// from the third build on (the parts are in the target's pack
			// only, written by the second), an unchanged part is read
			// whole (stage D 2e) and the edited function's is generated
			if s.name != "first" && s.name != "again" && !reused(log, "parts") {
				t.Errorf("%s %s: no part read from the cache, so the part cache is not tested:\n%s", s.name, target, log)
			}
			// from the third build on, functions whose inlining is read
			// from the cache are never decoded (stage D 2b)
			if s.name != "first" && s.name != "again" && !regexp.MustCompile(`cache rc [1-9]\d*/\d+ undecoded`).MatchString(log) {
				t.Errorf("%s %s: no function left undecoded, so the lazy path is not tested:\n%s", s.name, target, log)
			}
		}
		if s.name == "again" {
			// the second build of each target wrote its pack (decision
			// 0220 addendum stage D 2d); without the loose entries the
			// edit's build must still reuse functions, from the packs
			segs, _ := filepath.Glob(filepath.Join(work, "cache", "pack", "*", "*", "s-*"))
			if len(segs) < 4 {
				t.Fatalf("after the second builds: %d pack segments, want at least rc and fn for each target", len(segs))
			}
			for _, kind := range []string{"rc", "inl", "fn", "sum", "part", "fe"} {
				if err := os.RemoveAll(filepath.Join(work, "cache", kind)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

// TestFrontCacheInterfaceEdits: the front end cache (decision 0220
// addendum stage 2) keys a body by its module's interface, so an edit of
// a struct's field, of a callee's signature, or of a constant's value
// misses the bodies that use them, and the binary is the same file as a
// build with XO_BACKEND_CACHE=off after each edit; an edit inside one
// body leaves the others read from the cache.
func TestFrontCacheInterfaceEdits(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-fecache-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	type variant struct {
		name, field, sig, call, konst, body string
	}
	base := variant{"first", "", "x: Int", "callee(n)", "3", "n * 2"}
	gen := func(v variant) string {
		var b strings.Builder
		fmt.Fprintf(&b, "struct P {\n  a: Int\n%s}\n\nconst K = %s\n\n", v.field, v.konst)
		fmt.Fprintf(&b, "fn callee(%s) -> Int {\n  x + K\n}\n\n", v.sig)
		b.WriteString("fn use_p(p: P) -> Int {\n  p.a + 1\n}\n\n")
		fmt.Fprintf(&b, "fn use_callee(n: Int) -> Int {\n  %s\n}\n\n", v.call)
		fmt.Fprintf(&b, "fn body_only(n: Int) -> Int {\n  %s\n}\n\n", v.body)
		// a contract wraps its body (Job.Inner): never replayed
		b.WriteString("fn checked(n: Int) -> Int\n  requires n > 0\n  ensures result > n\n{\n  n + 1\n}\n\n")
		for i := 0; i < 40; i++ {
			fmt.Fprintf(&b, "fn g%d(x: Int) -> Int {\n  x + %d\n}\n\n", i, i)
		}
		b.WriteString("fn main(os: Os) {\n  var s = 0\n")
		for i := 0; i < 40; i++ {
			fmt.Fprintf(&b, "  s = s + g%d(s)\n", i)
		}
		b.WriteString("  os.stdio.println(\"${s} ${use_p(P{a: 4})} ${use_callee(5)} ${body_only(6)} ${checked(7)}\")\n}\n")
		return b.String()
	}
	build := func(t *testing.T, name string, env ...string) ([]byte, string) {
		out := filepath.Join(work, name, "prog")
		cmd := exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, ".")
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	hits := func(log string) (int, int) {
		m := regexp.MustCompile(`cache fe (\d+)/(\d+) bodies`).FindStringSubmatch(log)
		if m == nil {
			return -1, -1
		}
		h, _ := strconv.Atoi(m[1])
		n, _ := strconv.Atoi(m[2])
		return h, n
	}
	steps := []struct {
		v variant
		// all of the program's own bodies hit (want "all"), some (the
		// edited one missed: "some"), or the runtime's only ("std")
		want string
	}{
		{base, ""},
		{variant{"again", "", "x: Int", "callee(n)", "3", "n * 2"}, "all"},
		{variant{"body", "", "x: Int", "callee(n)", "3", "n * 3"}, "some"},
		{variant{"field", "  b: Int = 0\n", "x: Int", "callee(n)", "3", "n * 3"}, "std"},
		{variant{"signature", "  b: Int = 0\n", "x: Int, y: Int", "callee(n, 1)", "3", "n * 3"}, "std"},
		{variant{"const", "  b: Int = 0\n", "x: Int, y: Int", "callee(n, 1)", "4", "n * 3"}, "std"},
	}
	var full int
	for _, s := range steps {
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(gen(s.v)), 0o644); err != nil {
			t.Fatal(err)
		}
		on, log := build(t, "on", "XO_BACKEND_CACHE=on")
		off, _ := build(t, "off", "XO_BACKEND_CACHE=off")
		if !bytes.Equal(on, off) {
			t.Errorf("%s: the build through the front end cache differs from the build with XO_BACKEND_CACHE=off", s.v.name)
		}
		// verify builds every body and fails the build on a difference
		build(t, "verify", "XO_BACKEND_CACHE=verify")
		h, n := hits(log)
		if h < 0 {
			t.Fatalf("%s: no front end cache line:\n%s", s.v.name, log)
		}
		switch s.want {
		case "all":
			full = h
			if h < n-8 {
				t.Errorf("%s: %d of %d bodies from the cache, want nearly all", s.v.name, h, n)
			}
		case "some":
			if h >= full || h < full-8 {
				t.Errorf("%s: %d bodies from the cache, want a few fewer than %d (the edited body and those after it in its file)", s.v.name, h, full)
			}
		case "std":
			// the module's interface changed: none of its bodies hit (at
			// least the 40 g functions)
			if h > full-40 {
				t.Errorf("%s: %d bodies from the cache, want at most %d (every body of the edited module missed)", s.v.name, h, full-40)
			}
		}
	}
}
