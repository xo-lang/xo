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
	"sync"
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
// same file. Every program with XO_SELFHOST_TIER=full, else a sample.
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
	buildWith := func(bin string, i int, dir string, env ...string) ([]byte, error) {
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
