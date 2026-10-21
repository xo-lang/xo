package arm64

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
// body leaves the others read from the cache, also when the edited body
// is early in the build and makes a type no other body has (an entry
// holds no state of the builder, decision 0271: before, every later body
// missed).
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
		name, field, sig, call, konst, body, g0 string
	}
	base := variant{"first", "", "x: Int", "callee(n)", "3", "n * 2", ""}
	gen := func(v variant) string {
		var b strings.Builder
		fmt.Fprintf(&b, "struct P {\n  a: Int\n%s}\n\nconst K = %s\n\n", v.field, v.konst)
		fmt.Fprintf(&b, "fn callee(%s) -> Int {\n  x + K\n}\n\n", v.sig)
		b.WriteString("fn use_p(p: P) -> Int {\n  p.a + 1\n}\n\n")
		fmt.Fprintf(&b, "fn use_callee(n: Int) -> Int {\n  %s\n}\n\n", v.call)
		fmt.Fprintf(&b, "fn body_only(n: Int) -> Int {\n  %s\n}\n\n", v.body)
		// generic instances: the first use makes pick[Int], the second
		// reuses it and is recorded
		b.WriteString("fn pick[T](a: T, b: T, first: Bool) -> T {\n  if first { a } else { b }\n}\n\n")
		b.WriteString("fn gen_a(n: Int) -> Int {\n  pick(n, 1, true)\n}\n\n")
		b.WriteString("fn gen_b(n: Int) -> Int {\n  pick(n, 2, false)\n}\n\n")
		// a contract wraps its body (Job.Inner): never replayed
		b.WriteString("fn checked(n: Int) -> Int\n  requires n > 0\n  ensures result > n\n{\n  n + 1\n}\n\n")
		for i := 0; i < 40; i++ {
			if i == 0 && v.g0 != "" {
				fmt.Fprintf(&b, "fn g0(x: Int) -> Int {\n  %s\n}\n\n", v.g0)
				continue
			}
			fmt.Fprintf(&b, "fn g%d(x: Int) -> Int {\n  x + %d\n}\n\n", i, i)
		}
		b.WriteString("fn main(os: Os) {\n  var s = 0\n")
		for i := 0; i < 40; i++ {
			fmt.Fprintf(&b, "  s = s + g%d(s)\n", i)
		}
		b.WriteString("  os.stdio.println(\"${s} ${use_p(P{a: 4})} ${use_callee(5)} ${body_only(6)} ${checked(7)} ${gen_a(8)} ${gen_b(9)}\")\n}\n")
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
		{variant{"again", "", "x: Int", "callee(n)", "3", "n * 2", ""}, "all"},
		{variant{"body", "", "x: Int", "callee(n)", "3", "n * 3", ""}, "some"},
		// the first body built after main makes a tuple type no other
		// body has (one line, so no other body moves): the bodies built
		// after it still hit
		{variant{"newtype", "", "x: Int", "callee(n)", "3", "n * 3", "(x, true, \"g\").0"}, "some"},
		{variant{"field", "  b: Int = 0\n", "x: Int", "callee(n)", "3", "n * 3", ""}, "std"},
		{variant{"signature", "  b: Int = 0\n", "x: Int, y: Int", "callee(n, 1)", "3", "n * 3", ""}, "std"},
		{variant{"const", "  b: Int = 0\n", "x: Int, y: Int", "callee(n, 1)", "4", "n * 3", ""}, "std"},
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

// TestModuleCheckDiagnostics: a module whose check comes from the module
// check cache (decision 0220 addendum stage 2) reports the diagnostics of
// its last full check, so `xo build --json` prints the same object warm
// as cold, warning included, and after an edit of the module (checked
// again) the new ones.
func TestModuleCheckDiagnostics(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-chkcache-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	gen := func(k int) string {
		return fmt.Sprintf(`enum Shape {
  Circle(r: Int)
  Rect(w: Int, h: Int)
  Empty
}

fn area(s: Shape) -> Int {
  match s {
    Circle(r) => r * r * %d
    _ => 0
  }
}

fn main(os: Os) {
  os.stdio.println("${area(Shape.Circle(r: 2))} ${area(Shape.Empty)}")
}
`, k)
	}
	build := func(t *testing.T) (string, string) {
		cmd := exec.Command(xo, "build", "--backend=arm64", "--json", "--phase-times", "-o", filepath.Join(work, "prog"), ".")
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BACKEND_CACHE=on", "XO_CHK_CACHE=1")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("build: %v\n%s\n%s", err, out.String(), errb.String())
		}
		return out.String(), errb.String()
	}
	skipped := func(log string) bool {
		return regexp.MustCompile(`cache chk [1-9]\d*/\d+ modules`).MatchString(log)
	}
	for _, s := range []struct {
		name string
		k    int
		hit  bool
	}{{"cold", 3, false}, {"warm", 3, true}, {"edit", 4, false}, {"again", 4, true}} {
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(gen(s.k)), 0o644); err != nil {
			t.Fatal(err)
		}
		got, log := build(t)
		if skipped(log) != s.hit {
			t.Errorf("%s: module skipped %v, want %v:\n%s", s.name, skipped(log), s.hit, log)
		}
		if !strings.Contains(got, `"XO0305"`) {
			t.Errorf("%s: the warning is missing:\n%s", s.name, got)
		}
		cold := exec.Command(xo, "build", "--backend=arm64", "--json", "-o", filepath.Join(work, "prog-off"), ".")
		cold.Dir = src
		cold.Env = append(os.Environ(), "XO_BACKEND_CACHE=off")
		want, err := cold.Output()
		if err != nil {
			t.Fatal(err)
		}
		// the binaries' paths differ; the diagnostics must not
		norm := func(s string) string {
			return regexp.MustCompile(`"binaries": \{[^}]*\}`).ReplaceAllString(s, "")
		}
		if norm(got) != norm(string(want)) {
			t.Errorf("%s: the JSON through the cache differs from the cache-off build's:\n%s\nwant:\n%s", s.name, got, want)
		}
	}
	// verify checks every module and fails on an entry that differs
	var entries []string
	filepath.WalkDir(filepath.Join(work, "cache", "chk"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			entries = append(entries, p)
		}
		return nil
	})
	if len(entries) == 0 {
		t.Fatal("no chk entries stored")
	}
	verify := func() ([]byte, error) {
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", filepath.Join(work, "prog"), ".")
		cmd.Dir = src
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BACKEND_CACHE=verify", "XO_CHK_CACHE=1")
		return cmd.CombinedOutput()
	}
	if log, err := verify(); err != nil {
		t.Fatalf("verify: %v\n%s", err, log)
	}
	for _, e := range entries {
		f, err := os.OpenFile(e, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("x")
		f.Close()
	}
	if log, err := verify(); err == nil || !strings.Contains(string(log), "modules' check differs") {
		t.Errorf("verify with changed entries: %v, want the module mismatch:\n%s", err, log)
	}
}

// TestFrontCacheAbsentHelper: a body's entry records the function names
// it looked for and did not find (decision 0271 step 1a). fb removes from
// a Map[Str, Bytes], which lowers to Map[Str, Str] with a Bytes? result:
// alone, its helper is xo.map.remove[Map[Str, Str]]; after fa (a
// Map[Str, Str] with a Str? result) has that name, fb's helper takes a
// result suffix. fb's entry from the first build must not commit into the
// second, where the name is fa's other function: the binary is the same
// file as a build with XO_BACKEND_CACHE=off, and verify is clean.
func TestFrontCacheAbsentHelper(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-feabsent-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	funcs := "fn fb(n: Int) -> Int {\n  var m: Map[Str, Bytes] = {}\n  m[\"a\"] = b\"xy\"\n  let v = m.remove(\"a\") ?? b\"\"\n  v.len() + n\n}\n\n" +
		"fn fa(n: Int) -> Int {\n  var m: Map[Str, Str] = {}\n  m[\"a\"] = \"xyz\"\n  let v = m.remove(\"a\") ?? \"\"\n  v.len() + n\n}\n\n"
	build := func(name string, env ...string) []byte {
		out := filepath.Join(work, name, "prog")
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, body := range []string{"os.stdio.println(\"${fb(1)}\")", "os.stdio.println(\"${fa(1)} ${fb(1)}\")"} {
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(funcs+"fn main(os: Os) {\n  "+body+"\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// the first build after the edit commits the entries of the last
		on := build("on", "XO_BACKEND_CACHE=on")
		off := build("off", "XO_BACKEND_CACHE=off")
		if !bytes.Equal(on, off) {
			t.Errorf("%q: the build through the front end cache differs from the build with XO_BACKEND_CACHE=off", body)
		}
		build("verify", "XO_BACKEND_CACHE=verify")
	}
}

// TestFrontCacheAcrossPrograms: front end cache entries are shared by
// every program built with one cache (a std body's key does not name the
// program), so an entry recorded while building one program commits into
// another. The golden and run programs are built into a shared cache by 8
// builds at once (as `xo dev goldens -j 8` does), then again one at a time
// in three other orders (each program now after others' entries) and with
// the cache off, to one output path per program: each binary must be the
// same file. Every program with XO_SELFHOST_TIER=full, else a sample. The
// std modules with tests run their tests (xo test --std-module) among
// them, each pass of each order: a std module under test is the build's
// root, so its functions are named as main's (2026-10-20: a std body
// recorded there called "svc" for stop.svc in other builds).
func TestFrontCacheAcrossPrograms(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-fexprog-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	runs, err := filepath.Glob(filepath.Join("testdata", "run", "*"))
	if err != nil {
		t.Fatal(err)
	}
	all := append(goldenDirs(t), runs...)
	var dirs []string
	for i, d := range all {
		if _, err := os.Stat(filepath.Join(d, "main.xo")); err != nil {
			continue
		}
		if os.Getenv("XO_SELFHOST_TIER") == "full" || i%6 == 0 {
			dirs = append(dirs, d)
		}
	}
	// XO_TEST_WARM_XO (scripts/gate.sh warm, decision 0275): the first
	// pass builds with that xo (the parent commit's), so the orders read
	// entries an earlier xo wrote, as a cache keyed on the code generation
	// key shares them across xo builds
	warm := os.Getenv("XO_TEST_WARM_XO")
	if warm == "" {
		warm = xo
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	filepath.WalkDir(filepath.Join(root, "std"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, "_test.xo") && !strings.Contains(p, string(filepath.Separator)+"testdata"+string(filepath.Separator)) {
			rel, _ := filepath.Rel(filepath.Join(root, "std"), p)
			dirs = append(dirs, "std:"+strings.TrimSuffix(filepath.ToSlash(rel), "_test.xo"))
		}
		return nil
	})
	buildWith := func(bin string, i int, dir string, env ...string) ([]byte, error) {
		if m, ok := strings.CutPrefix(dir, "std:"); ok {
			// a std module's tests, on that xo's own std (bin/../std): they
			// must pass (no binary to compare)
			std := filepath.Join(filepath.Dir(filepath.Dir(bin)), "std")
			cmd := exec.Command(bin, "test", "--std-module="+m, "--backend=arm64")
			cmd.Dir = root
			cmd.Env = append(append(os.Environ(), "XO_STD="+std, "PWD="+root, "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
			if log, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("%v\n%s", err, log)
			}
			return nil, nil
		}
		out := filepath.Join(work, "out", strconv.Itoa(i), "prog")
		os.Remove(out)
		cmd := exec.Command(bin, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%v\n%s", err, log)
		}
		return os.ReadFile(out)
	}
	build := func(i int, dir string, env ...string) ([]byte, error) { return buildWith(xo, i, dir, env...) }
	// the first pass: 8 builds at once
	next := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first []string
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				if _, err := buildWith(warm, i, dirs[i], "XO_BACKEND_CACHE=on"); err != nil {
					mu.Lock()
					first = append(first, fmt.Sprintf("%s: %v", filepath.Base(dirs[i]), err))
					mu.Unlock()
				}
			}
		}()
	}
	for i := range dirs {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, f := range first {
		t.Errorf("first pass: %s", f)
	}
	n := len(dirs)
	orders := [][]int{{}, {}, {}}
	for i := 0; i < n; i++ {
		orders[0] = append(orders[0], n-1-i)
		orders[2] = append(orders[2], (i+n/2)%n)
	}
	for i := 1; i < n; i += 2 {
		orders[1] = append(orders[1], i)
	}
	for i := 0; i < n; i += 2 {
		orders[1] = append(orders[1], i)
	}
	for k, order := range orders {
		for _, i := range order {
			d := dirs[i]
			on, err := build(i, d, "XO_BACKEND_CACHE=on")
			if err != nil {
				t.Errorf("order %d, %s through the cache: %v", k, filepath.Base(d), err)
				continue
			}
			off, err := build(i, d, "XO_BACKEND_CACHE=off")
			if err != nil {
				t.Fatalf("%s: %v", filepath.Base(d), err)
			}
			if !bytes.Equal(on, off) {
				t.Errorf("order %d, %s: the build through the shared cache differs from the build with XO_BACKEND_CACHE=off", k, filepath.Base(d))
			}
		}
	}
}

// TestFrontCacheOneWayFlags guards the entry format: a body that sets one
// of the builder's one-way flags (uses_net here, need_conc alike) records
// the set, even when the recording builder had the flag already (decision
// 0271 step 1a). b dials, and so did a before it; after an edit a no
// longer dials, and the binary is the same file as a build with
// XO_BACKEND_CACHE=off, and verify is clean. A serial build passed this
// before the fix too (the commit's declares mark the network's modules,
// and the task scan sets need_conc before any body): the masking broke
// bodies recorded in other builders (step 2's prebuild).
func TestFrontCacheOneWayFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-feflags-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dial := "match net.dial(\"127.0.0.1:1\") {\n    Ok(_) => 1\n    Err(_) => 0\n  }"
	gen := func(a string) string {
		return "fn a(net: Net) -> Int {\n  " + a + "\n}\n\nfn b(net: Net) -> Int {\n  " + dial + "\n}\n\n" +
			"fn main(os: Os) {\n  os.stdio.println(\"${a(os.net)} ${b(os.net)}\")\n}\n"
	}
	build := func(name string, env ...string) []byte {
		out := filepath.Join(work, name, "prog")
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = src
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for i, a := range []string{dial, "0"} {
		if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(gen(a)), 0o644); err != nil {
			t.Fatal(err)
		}
		// the first build after the edit commits the entries of the last
		on := build("on", "XO_BACKEND_CACHE=on")
		off := build("off", "XO_BACKEND_CACHE=off")
		if !bytes.Equal(on, off) {
			t.Errorf("step %d: the build through the front end cache differs from the build with XO_BACKEND_CACHE=off", i)
		}
		build("verify", "XO_BACKEND_CACHE=verify")
	}
}

// TestFrontCacheStamps: a body that reads a stamped constant (--stamp,
// decision 0264) is keyed by the stamp, so a second build with another
// value in the same cache prints the new value (the front end cache keyed
// it by the source text alone and replayed the first value; a seed's
// `xo version` once named another build's commit). Native and LLVM.
func TestFrontCacheStamps(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-festamp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	prog := "const VERSION: Str = \"dev\"\n\nfn line() -> Str { \"version ${VERSION}\" }\n\nfn main(os: Os) {\n  os.stdio.println(line())\n}\n"
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"arm64", "llvm"} {
		for _, v := range []string{"one", "two", "one"} {
			out := filepath.Join(work, backend, v, "prog")
			cmd := exec.Command(xo, "build", "--backend="+backend, "--stamp", "VERSION="+v, "-o", out, ".")
			cmd.Dir = src
			cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"))
			if log, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s %s: %v\n%s", backend, v, err, log)
			}
			got, err := exec.Command(out).Output()
			if err != nil {
				t.Fatalf("%s %s: run: %v", backend, v, err)
			}
			if want := "version " + v + "\n"; string(got) != want {
				t.Errorf("%s, --stamp VERSION=%s: printed %q, want %q", backend, v, got, want)
			}
		}
	}
}

// TestFrontCacheEntryDoesNotReplay: an entry whose checks pass and whose
// commit fails (XO_FE_BREAK forces it for one function) does not fail
// the build: the build is made again without the entry, with a warning
// naming it, and the binary is the cache-off build's
// (gotchas/fe-cache-entry-does-not-replay.md).
func TestFrontCacheEntryDoesNotReplay(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-fenoreplay-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	dir := filepath.Join(work, "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn twice(x: Int) -> Int {\n  x * 2\n}\n\nfn main(os: Os) uses stdio {\n  os.stdio.println(\"${twice(21)}\")\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "prog")
	build := func(env ...string) ([]byte, string) {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache")), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %v: %v\n%s", env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	build("XO_BACKEND_CACHE=on")
	broken, log := build("XO_BACKEND_CACHE=on", "XO_FE_BREAK=twice")
	if !strings.Contains(log, "did not replay") || !strings.Contains(log, "(twice, recorded by xo ") {
		t.Fatalf("the build with a broken entry printed no warning naming it:\n%s", log)
	}
	if kept, _ := filepath.Glob(filepath.Join(work, "cache", "fe-bad", "*")); len(kept) != 1 {
		t.Fatalf("the broken entry was not kept under the cache's fe-bad: %v", kept)
	}
	off, _ := build("XO_BACKEND_CACHE=off")
	if !bytes.Equal(broken, off) {
		t.Fatal("the build made again without the broken entry differs from the build with XO_BACKEND_CACHE=off")
	}
	again, log := build("XO_BACKEND_CACHE=on")
	if strings.Contains(log, "did not replay") || !bytes.Equal(again, off) {
		t.Fatalf("the build after the fallback warned again or differs from the cache-off build:\n%s", log)
	}
}

// TestModuleCheckAuto: XO_CHK_CACHE=auto skips a module's check only when
// every body of it has a front end cache entry, so no pass misses a body
// built from source (gotchas/module-check-cache-skips-whole-program-
// passes.md). Each program (a sample of the goldens and os_tasks, whose
// task closure lost its error under XO_CHK_CACHE=1) is built twice in one
// cache with auto, and again after the front end cache's entries are
// removed (the modules are skipped and their bodies built from source:
// the fallback checks them again, with a warning); every build's binary
// is the cache-off build's.
func TestModuleCheckAuto(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-chkauto-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	dirs := []string{filepath.Join(root, "internal", "gen", "testdata", "golden", "os_tasks")}
	for i, d := range goldenDirs(t) {
		if _, err := os.Stat(filepath.Join(d, "main.xo")); err == nil && (os.Getenv("XO_SELFHOST_TIER") == "full" || i%8 == 0) {
			dirs = append(dirs, d)
		}
	}
	out := filepath.Join(work, "prog")
	build := func(dir, cache string, env ...string) ([]byte, string) {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: xo build %v: %v\n%s", filepath.Base(dir), env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	skipped := regexp.MustCompile(`cache chk ([1-9]\d*)/\d+ modules`)
	nskip := 0
	for i, d := range dirs {
		cache := filepath.Join(work, "cache", strconv.Itoa(i))
		off, _ := build(d, cache, "XO_BACKEND_CACHE=off")
		auto := []string{"XO_BACKEND_CACHE=on", "XO_CHK_CACHE=auto"}
		build(d, cache, auto...)
		warm, log := build(d, cache, auto...)
		if strings.Contains(log, "module check cache skipped") {
			t.Errorf("%s: the warm build fell back:\n%s", filepath.Base(d), log)
		}
		if !bytes.Equal(warm, off) {
			t.Errorf("%s: the warm build with XO_CHK_CACHE=auto differs from the cache-off build", filepath.Base(d))
		}
		if skipped.MatchString(log) {
			nskip++
			// the front end cache's entries gone, the module entries kept:
			// the skipped modules' bodies are built from source. A golden's
			// one module has no body the front end cache replays (its root
			// bodies are on its entry's eager list, checked before the
			// whole-program passes), so it need not fall back;
			// TestModuleCheckAutoGeneric has a module that must
			removeFeEntries(cache)
			fb, _ := build(d, cache, auto...)
			if !bytes.Equal(fb, off) {
				t.Errorf("%s: the build after the fallback differs from the cache-off build", filepath.Base(d))
			}
		}
	}
	if nskip == 0 {
		t.Error("XO_CHK_CACHE=auto skipped no module of any program")
	}
}

// TestFrontCacheClosures: a body that makes closures is cached with its
// closures' bodies (decision 0271 step 1b, the closure relocation): a
// second build replays them, renumbered as that build numbers closures,
// and is the cache-off build's binary; verify builds every body, closures
// too, and compares. Programs: nested and capturing closures, and
// conc_mutex_rollback's `m.with(fn(var l) ...)` (closure_with).
func TestFrontCacheClosures(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-feclos-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `fn adder(n: Int) -> fn(Int) -> Int {
  fn(x: Int) -> Int { x + n }
}

fn twice(f: fn(Int) -> Int) -> fn(Int) -> Int {
  fn(x: Int) -> Int {
    let g = fn(y: Int) -> Int { f(f(y)) }
    g(x)
  }
}

fn main(os: Os) uses stdio {
  let k = 3
  let h = twice(adder(k))
  let xs = [1, 2, 3].map(fn(x) { h(x) * k })
  os.stdio.println("${xs[0]} ${xs[1]} ${xs[2]}")
}
`
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs := []string{own, filepath.Join("..", "llvm", "testdata", "golden", "conc_mutex_rollback")}
	out := filepath.Join(work, "prog")
	build := func(dir, cache string, env ...string) []byte {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: xo build %v: %v\n%s", filepath.Base(dir), env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for i, d := range dirs {
		cache := filepath.Join(work, "cache", strconv.Itoa(i))
		off := build(d, cache, "XO_BACKEND_CACHE=off")
		build(d, cache, "XO_BACKEND_CACHE=on")
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=on"), off) {
			t.Errorf("%s: the build replaying closures differs from the cache-off build", filepath.Base(d))
		}
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=verify"), off) {
			t.Errorf("%s: the verify build differs from the cache-off build", filepath.Base(d))
		}
	}
	// closures numbered differently: a function made first now makes a
	// closure, so every replayed closure gets another name than the one
	// it was recorded under (as when a std body's entry from another
	// program replays)
	first := "fn first(n: Int) -> Int {\n  let f = fn(x: Int) -> Int { x + n }\n  f(1)\n}\n\n"
	src2 := strings.Replace(src, "fn main(os: Os) uses stdio {\n", "fn main(os: Os) uses stdio {\n  let _ = first(2)\n", 1)
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(first+src2), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(work, "cache", "0")
	off := build(own, filepath.Join(work, "cache", "off"), "XO_BACKEND_CACHE=off")
	if !bytes.Equal(build(own, cache, "XO_BACKEND_CACHE=on"), off) {
		t.Error("the build replaying renumbered closures differs from the cache-off build")
	}
}

// TestFrontCacheConsts: bodies that read constants of collection types
// are cached, the constants numbered at a commit as the build there
// numbers them (decision 0271's consts relocation).
func TestFrontCacheConsts(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-feconst-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `const PRIMES = [2, 3, 5, 7, 11]
const NAMES: List[Str] = ["a", "b", "c"]
const WORDS = {"if": 1, "else": 2}

fn sum() -> Int {
  var s = 0
  for p in PRIMES {
    s += p
  }
  s + PRIMES.len()
}

fn label(i: Int) -> Str {
  "${NAMES[i % NAMES.len()]}${WORDS.get("else") ?? 0}"
}

fn main(os: Os) uses stdio {
  os.stdio.println("${sum()} ${label(4)} ${PRIMES[2]} ${WORDS.len()}")
}
`
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs := []string{own, filepath.Join("..", "llvm", "testdata", "golden", "native_bytes")}
	out := filepath.Join(work, "prog")
	build := func(dir, cache string, env ...string) []byte {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: xo build %v: %v\n%s", filepath.Base(dir), env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for i, d := range dirs {
		cache := filepath.Join(work, "cache", strconv.Itoa(i))
		off := build(d, cache, "XO_BACKEND_CACHE=off")
		build(d, cache, "XO_BACKEND_CACHE=on")
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=on"), off) {
			t.Errorf("%s: the build replaying constants differs from the cache-off build", filepath.Base(d))
		}
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=verify"), off) {
			t.Errorf("%s: the verify build differs from the cache-off build", filepath.Base(d))
		}
	}
	// constants numbered differently: a function built first now reads a
	// constant no other body reads, so every replayed body's constants
	// get other numbers than the ones they were recorded under
	first := "const FIRST = [9, 8]\n\nfn first() -> Int {\n  FIRST[1]\n}\n\n"
	src2 := strings.Replace(src, "fn main(os: Os) uses stdio {\n", "fn main(os: Os) uses stdio {\n  let _ = first()\n", 1)
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(first+src2), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(work, "cache", "0")
	off := build(own, filepath.Join(work, "cache", "off"), "XO_BACKEND_CACHE=off")
	if !bytes.Equal(build(own, cache, "XO_BACKEND_CACHE=on"), off) {
		t.Error("the build replaying renumbered constants differs from the cache-off build")
	}
	// another body makes a constant first: a library's sum, whose entry
	// made PRIMES (and its list type's drop), replays with peek's (the
	// same functions, by their names) instead of missing, which made
	// every warm edit of xo fall back under XO_CHK_CACHE=auto
	// (2026-10-21). A library: the root module's bodies have no entries.
	lp := filepath.Join(work, "lp")
	if err := os.MkdirAll(filepath.Join(lp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := "const PRIMES = [2, 3, 5, 7, 11]\n\npub fn peek() -> Int {\n  PRIMES[0]\n}\n\npub fn sum() -> Int {\n  var s = 0\n  for p in PRIMES {\n    s += p\n  }\n  s\n}\n"
	for p, s := range map[string]string{"xo.mod": "module example.com/p\n", "lib/lib.xo": lib} {
		if err := os.WriteFile(filepath.Join(lp, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainOf := func(call string) []byte {
		return []byte("use example.com/p/lib\n\nfn main(os: Os) uses stdio {\n  os.stdio.println(\"${" + call + "}\")\n}\n")
	}
	lcache := filepath.Join(work, "cache", "lib")
	stats := func() string {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = lp
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+lcache, "XO_BACKEND_CACHE=on", "XO_FE_STATS=1")
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build: %v\n%s", err, log)
		}
		return string(log)
	}
	if err := os.WriteFile(filepath.Join(lp, "main.xo"), mainOf("lib.sum()"), 0o644); err != nil {
		t.Fatal(err)
	}
	stats()
	if err := os.WriteFile(filepath.Join(lp, "main.xo"), mainOf("lib.peek()} ${lib.sum()"), 0o644); err != nil {
		t.Fatal(err)
	}
	if log := stats(); strings.Contains(log, "fe miss.const") || strings.Contains(log, "fe miss.absent") {
		t.Errorf("sum missed over a constant peek made first:\n%s", log)
	}
	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, build(lp, filepath.Join(work, "cache", "liboff"), "XO_BACKEND_CACHE=off")) {
		t.Error("the build replaying a constant another body made differs from the cache-off build")
	}
}

// TestFrontCacheSitesThunks: bodies that spawn tasks that can fail and
// bodies that take functions as values are cached, the spawn sites
// numbered and the thunks found or made at a commit as the build there
// does (decision 0271's sites and thunks relocations).
func TestFrontCacheSitesThunks(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-fesite-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `enum JobErr {
  Bad(id: Int)
}

fn job(id: Int) -> Int ! JobErr {
  if id == 2 {
    return Err(JobErr.Bad(id: id))
  }
  id * 10
}

fn double(x: Int) -> Int { x * 2 }

fn inc(x: Int) -> Int { x + 1 }

fn run(n: Int) -> Int ! JobErr {
  scope s {
    s.spawn(fn() { job(n) })
    s.spawn(fn() { job(n + 3) })
  }
  n
}

fn show(r: Result[Int, JobErr]) -> Str {
  match r {
    Ok(v) => "ok ${v}"
    Err(e) => match e {
      Bad(id) => "bad ${id}"
    }
  }
}

fn main(os: Os) uses stdio {
  let f = double
  let xs = [1, 2, 3].map(inc)
  os.stdio.println("${f(4)} ${xs[2]} ${[5].map(double)[0]}")
  os.stdio.println("${show(run(4))} ${show(run(2))}")
}
`
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs := []string{own, filepath.Join("..", "llvm", "testdata", "golden", "conc_scopes")}
	out := filepath.Join(work, "prog")
	build := func(dir, cache string, env ...string) []byte {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", out, ".")
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: xo build %v: %v\n%s", filepath.Base(dir), env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for i, d := range dirs {
		cache := filepath.Join(work, "cache", strconv.Itoa(i))
		off := build(d, cache, "XO_BACKEND_CACHE=off")
		build(d, cache, "XO_BACKEND_CACHE=on")
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=on"), off) {
			t.Errorf("%s: the build replaying sites and thunks differs from the cache-off build", filepath.Base(d))
		}
		if !bytes.Equal(build(d, cache, "XO_BACKEND_CACHE=verify"), off) {
			t.Errorf("%s: the verify build differs from the cache-off build", filepath.Base(d))
		}
	}
	// numbered differently: a function built first now spawns and makes
	// a thunk, so the replayed bodies' sites get other numbers and their
	// thunks are found rather than made
	first := "fn first() -> Int ! JobErr {\n  let g = inc\n  scope s {\n    s.spawn(fn() { job(g(0)) })\n  }\n  0\n}\n\n"
	src2 := strings.Replace(src, "fn main(os: Os) uses stdio {\n", "fn main(os: Os) uses stdio {\n  os.stdio.println(show(first()))\n", 1)
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(first+src2), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(work, "cache", "0")
	off := build(own, filepath.Join(work, "cache", "off"), "XO_BACKEND_CACHE=off")
	if !bytes.Equal(build(own, cache, "XO_BACKEND_CACHE=on"), off) {
		t.Error("the build replaying renumbered sites and found thunks differs from the cache-off build")
	}
}

// TestModuleCheckAutoGeneric: XO_CHK_CACHE=auto skips a module whose
// bodies include a generic function (always built from source): its
// entry lists the function and the inferred function its body reads
// (ModEntry.eager), checked before the whole-program passes, so the
// warm build and a build after an edit of another module skip it with
// no fallback and are the cache-off builds' binaries.
func TestModuleCheckAutoGeneric(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-chkgen-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(filepath.Join(own, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"xo.mod": "module example.com/p\n",
		"lib/lib.xo": `fn clamp(i: Int, n: Int) -> Int {
  if i < 0 { 0 } else if i >= n { n - 1 } else { i }
}

pub fn pick[T](xs: List[T], i: Int) -> T {
  xs[clamp(i, xs.len())]
}

pub fn total(xs: List[Int]) -> Int {
  var s = 0
  for x in xs {
    s += x
  }
  s
}
`,
		"main.xo": `use example.com/p/lib

fn main(os: Os) uses stdio {
  let xs = [4, 5, 6]
  os.stdio.println("v1 ${lib.pick(xs, 9)} ${lib.pick(["a", "b"], -1)} ${lib.total(xs)}")
}
`,
	}
	for p, s := range files {
		if err := os.WriteFile(filepath.Join(own, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(work, "prog")
	build := func(cache string, env ...string) ([]byte, string) {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, ".")
		cmd.Dir = own
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %v: %v\n%s", env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	skipped := regexp.MustCompile(`cache chk ([1-9]\d*)/\d+ modules`)
	cache := filepath.Join(work, "cache")
	auto := []string{"XO_BACKEND_CACHE=on", "XO_CHK_CACHE=auto"}
	for _, v := range []string{"v1", "v2"} {
		if v == "v2" {
			// an edit of main's module only: lib's entry still applies
			m := strings.Replace(files["main.xo"], "v1 ", "v2 ", 1)
			if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(m), 0o644); err != nil {
				t.Fatal(err)
			}
		} else {
			build(cache, auto...)
		}
		off, _ := build(filepath.Join(work, "off"), "XO_BACKEND_CACHE=off")
		warm, log := build(cache, auto...)
		if strings.Contains(log, "module check cache skipped") {
			t.Errorf("%s: the build fell back:\n%s", v, log)
		}
		if !skipped.MatchString(log) {
			t.Errorf("%s: no module skipped:\n%s", v, log)
		}
		if !bytes.Equal(warm, off) {
			t.Errorf("%s: the build with XO_CHK_CACHE=auto differs from the cache-off build", v)
		}
	}
	// the front end cache's entries gone: lib's total, which its entry
	// left to check lazily, is built from source after the whole-program
	// passes, so the build falls back and checks lib again
	removeFeEntries(cache)
	fb, log := build(cache, auto...)
	if !strings.Contains(log, "module check cache skipped") {
		t.Errorf("no fallback with the front end cache's entries removed:\n%s", log)
	}
	off, _ := build(filepath.Join(work, "off"), "XO_BACKEND_CACHE=off")
	if !bytes.Equal(fb, off) {
		t.Error("the build after the fallback differs from the cache-off build")
	}
}

// removeFeEntries removes the front end cache's entries (every fe
// directory) from the back-end cache dir, keeping the module entries.
func removeFeEntries(cache string) {
	filepath.WalkDir(cache, func(p string, e fs.DirEntry, err error) error {
		if err == nil && e.IsDir() && e.Name() == "fe" {
			os.RemoveAll(p)
			return filepath.SkipDir
		}
		return nil
	})
}

// TestPruneBudget: a build that falls on an overdue prune of the
// back-end cache prunes for its budget only (XO_CACHE_PRUNE_BUDGET, 2 s
// by default; 1ms here, so the stop shows) and leaves the rest for the
// next build, which goes on: a build of one golden once timed out in
// the middle of a prune of a busy day's cache (2026-10-21).
func TestPruneBudget(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-prunebudget-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cache := filepath.Join(work, "cache")
	old := time.Now().Add(-10 * 24 * time.Hour)
	stale := filepath.Join(cache, "zz")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	const n = 20000
	for i := 0; i < n; i++ {
		p := filepath.Join(stale, strconv.Itoa(i))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	stamp := filepath.Join(cache, "prune-stamp")
	if err := os.WriteFile(stamp, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stamp, old, old); err != nil {
		t.Fatal(err)
	}
	left := func() int {
		es, _ := os.ReadDir(stale)
		return len(es)
	}
	dir := filepath.Join(root, "internal", "gen", "testdata", "golden", "os_tasks")
	build := func() time.Duration {
		t.Helper()
		t0 := time.Now()
		cmd := exec.Command(xo, "build", "--backend=arm64", "-o", filepath.Join(work, "prog"), ".")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache, "XO_BACKEND_CACHE=on", "XO_CACHE_PRUNE_BUDGET=1ms")
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build: %v\n%s", err, log)
		}
		return time.Since(t0)
	}
	d1 := build()
	n1 := left()
	if n1 == 0 || n1 == n {
		t.Fatalf("first build: %d of %d stale entries left, want some removed and some left (its prune stopped at the budget)", n1, n)
	}
	// the stopped prune left the stamp due a minute on (prune
	// PARTIAL_PAUSE): the next build, after that minute, goes on
	if err := os.Chtimes(stamp, old, old); err != nil {
		t.Fatal(err)
	}
	build()
	if n2 := left(); n2 >= n1 {
		t.Errorf("second build: %d stale entries left after %d, want the prune to go on", n2, n1)
	}
	t.Logf("first build %v, %d of %d stale entries left", d1, n1, n)
}

// TestModuleCheckAutoZones: a skipped module's eagerly checked bodies
// that call time.zone keep their names' resolution: replaying the
// module's entry removed their zone records, which shifted the indexes
// name_at holds, so their names resolved to other objects and the warm
// build failed with XO1201 (2026-10-21, the tz_table golden).
func TestModuleCheckAutoZones(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-chkzones-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `use std/time

const NAMES: List[Str] = ["Europe/Berlin", "Asia/Kathmandu"]

fn zone_of(name: Str) -> time.Zone {
  match name {
    "Europe/Berlin" => time.zone("Europe/Berlin")
    _ => time.zone("Asia/Kathmandu")
  }.ok() ?? time.utc()
}

fn main(os: Os) {
  for name in NAMES {
    let z = zone_of(name)
    os.stdio.println("${name} ${z}")
  }
}
`
	if err := os.WriteFile(filepath.Join(own, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "prog")
	build := func(cache string, env ...string) ([]byte, string) {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, ".")
		cmd.Dir = own
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+cache), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build %v: %v\n%s", env, err, log)
		}
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b, string(log)
	}
	off, _ := build(filepath.Join(work, "off"), "XO_BACKEND_CACHE=off")
	cache := filepath.Join(work, "cache")
	auto := []string{"XO_BACKEND_CACHE=on", "XO_CHK_CACHE=auto"}
	build(cache, auto...)
	warm, log := build(cache, auto...)
	if !regexp.MustCompile(`cache chk 1/1 modules`).MatchString(log) {
		t.Fatalf("the warm build skipped no module:\n%s", log)
	}
	if !bytes.Equal(warm, off) {
		t.Error("the warm build with XO_CHK_CACHE=auto differs from the cache-off build")
	}
}

// TestFrontCacheRootThenLibrary: a module's bodies cached while it was
// the build's root (`xo test` of it: its functions named unprefixed) are
// not replayed where it is a library (its functions named with its
// module): an entry `xo test compiler/ssa` recorded called
// value_self_faults by a name the build of compiler/arm64's tests lacked
// (2026-10-21). The front end cache keys a program module's body by
// whether its module is the root.
func TestFrontCacheRootThenLibrary(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-feroot-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	own := filepath.Join(work, "p")
	if err := os.MkdirAll(filepath.Join(own, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"xo.mod":          "module example.com/p\n",
		"lib/a.xo":        "pub fn api(xs: List[Int]) -> Int {\n  var n = 0\n  for x in xs {\n    if helper(x) {\n      n += 1\n    }\n  }\n  n\n}\n",
		"lib/b.xo":        "fn helper(x: Int) -> Bool {\n  x > 1\n}\n",
		"lib/lib_test.xo": "test \"api counts\" {\n  expect(api([1, 2, 3])) == 2\n}\n",
		"main.xo":         "use example.com/p/lib\n\nfn main(os: Os) uses stdio {\n  os.stdio.println(\"${lib.api([1, 2, 3, 4])}\")\n}\n",
	}
	for p, s := range files {
		if err := os.WriteFile(filepath.Join(own, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(work, "cache")
	run := func(env []string, args ...string) string {
		t.Helper()
		cmd := exec.Command(xo, args...)
		cmd.Dir = own
		cmd.Env = append(os.Environ(), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo %v %v: %v\n%s", args, env, err, log)
		}
		return string(log)
	}
	on := []string{"XO_BACKEND_CACHE_DIR=" + cache, "XO_BACKEND_CACHE=on"}
	// lib as the root, then as a library
	run(on, "test", "--backend=arm64", "./lib")
	out := filepath.Join(work, "prog")
	run(on, "build", "--backend=arm64", "-o", out, ".")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// the same output path (the code signature names the file)
	run([]string{"XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "off"), "XO_BACKEND_CACHE=off"}, "build", "--backend=arm64", "-o", out, ".")
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("the build after lib's test build differs from the cache-off build")
	}
}
