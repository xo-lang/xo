package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestSepcompLinks (decision 0388 stage 3): std built once on its own
// (xo dev stdcache: its store and facts), and a program built against it
// (XO_SEPCOMP: std's functions externs, std linked from its store)
// runs as the program built whole does, before and after an edit; the
// whole program's own std facts verify against std's (XO_STD_FACTS_VERIFY).
// The program passes a closure to std, has a by-reference var parameter
// of its own, and calls std's list, string and sort functions. A second,
// concurrent program (std's conc1 variant) passes a function constant to
// std (debug.router: specialize_args once cloned the extern, a body with
// no instructions, 2026-10-28).
func TestSepcompLinks(t *testing.T) {
	t.Parallel()
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
	work, err := testtmp.Dir("xo-sepcomp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	build := func(dir, out string, env ...string) {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", out, "."))
		cmd.Dir = dir
		// the cache on as outside a gate (gate.sh exports it off;
		// gotchas/test-inherits-gate-cache-off.md)
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE=on"), env...)
		if log, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s (%s): %v\n%s", dir, strings.Join(env, " "), err, log)
		}
	}
	run := func(bin string) string {
		t.Helper()
		out, err := testtmp.Guard(exec.Command(bin)).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", bin, err, out)
		}
		return string(out)
	}
	write := func(dir, src string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// std's store and facts through the std cache (xo dev stdcache: built
	// once per xo, target and concurrency, decision 0388)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	src := `struct Big {
  a: Int
  b: Int
  c: Int
  d: Int
  e: Int
  f: Int
  g: Int
  h: Int
  i: Int
  j: Int
  k: Int
  l: Int
  m: Int
  n: Int
  o: Int
  p: Int
  q: Int
  r: Int
  s: Int
  t: Int
  u: Int
  v: Int
  w: Int
  x: Int
  y: Int
  z: Int
  aa: Int
  bb: Int
  cc: Int
  dd: Int
  ee: Int
  ff: Int
  gg: Int
  hh: Int
}

fn bump(var s: Big, n: Int) {
  for i in 0..n {
    s.a = s.a + i
    s.hh = s.hh + s.a
  }
}

fn main(os: Os) {
  var xs: List[Int] = []
  for i in 0..20 {
    xs.push((i * 7) % 13)
  }
  let ys = xs.sort_by(fn(x) { -x })
  var s = Big{a: 1, b: 2, c: 3, d: 4, e: 5, f: 6, g: 7, h: 8, i: 9, j: 10, k: 11, l: 12, m: 13, n: 14, o: 15, p: 16, q: 17, r: 18, s: 19, t: 20, u: 21, v: 22, w: 23, x: 24, y: 25, z: 26, aa: 27, bb: 28, cc: 29, dd: 30, ee: 31, ff: 32, gg: 33, hh: 34}
  bump(s, 10)
  let words = "edit 0 of the program".split(" ")
  os.stdio.println("${ys.map(fn(y) { "${y}" }).join(",")} ${s.a} ${s.hh} ${words.len()} ${words[1].to_upper()}")
}
`
	prog := filepath.Join(work, "prog")
	write(prog, src)
	sep := []string{"XO_SEPCOMP=1", "XO_SEPCOMP_FACTS=" + facts, "XO_SEPCOMP_STORE=" + store, "XO_BACKEND_CACHE_DIR=" + filepath.Join(work, "c-sep")}
	for edit := 0; edit < 2; edit++ {
		if edit > 0 {
			write(prog, strings.Replace(src, "edit 0", "edit 1", 1))
		}
		whole := filepath.Join(work, "whole", "prog")
		build(prog, whole, "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-whole"), "XO_STD_FACTS_VERIFY="+facts)
		sepc := filepath.Join(work, "sep", "prog")
		build(prog, sepc, sep...)
		want, got := run(whole), run(sepc)
		if got != want {
			t.Fatalf("edit %d: the separately compiled program prints %q, the whole program %q", edit, got, want)
		}
	}
	// the concurrent variant: a facts directory (conc0.stdf, conc1.stdf)
	// and both stores, as the default sets them
	cmd = testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=1"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths1, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache --conc=1: %v", err)
	}
	lines1 := strings.Fields(string(paths1))
	if len(lines1) != 2 {
		t.Fatalf("xo dev stdcache --conc=1 printed %q, want the store and the facts", paths1)
	}
	fdir := filepath.Join(work, "facts")
	if err := os.MkdirAll(fdir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]string{"conc0.stdf": facts, "conc1.stdf": lines1[1]} {
		if err := os.Symlink(f, filepath.Join(fdir, name)); err != nil {
			t.Fatal(err)
		}
	}
	prog2 := filepath.Join(work, "prog2")
	write(prog2, `use std/debug
use std/http

fn allow(_req: http.Request) -> Bool { true }

fn main(os: Os) {
  let r = debug.router(os.clock, allow)
  os.stdio.println("${r.call(http.new_request("GET", "/nothing")).status} ${r.call(http.new_request("GET", "/sched")).status}")
}
`)
	whole := filepath.Join(work, "whole", "prog2")
	build(prog2, whole, "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-whole"))
	sepc := filepath.Join(work, "sep", "prog2")
	build(prog2, sepc, "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+fdir, "XO_SEPCOMP_STORE="+store, "XO_SEPCOMP_STORE_CONC="+lines1[0], "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-sep"))
	if want, got := run(whole), run(sepc); got != want {
		t.Fatalf("debug.router: the separately compiled program prints %q, the whole program %q", got, want)
	}
}

// TestSepcompDefault (decision 0388 stage 3, step d): with the default on
// (on by default; XO_SEPCOMP_DEFAULT=0 turns it off), a build for the host takes
// std's variants from the std cache and links separately (its phase line
// says so), runs as the whole program (XO_SEPCOMP=0) does, and
// XO_SEPCOMP=0 is never answered from the separately compiled entries.
func TestSepcompDefault(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's variants")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepdef-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn main(os: Os) {\n  let ws = \"a b c\".split(\" \")\n  os.stdio.println(\"${ws.len()} ${ws[2].to_upper()}\")\n}\n"
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(out string, env ...string) string {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "--phase-times", "-o", out, "."))
		cmd.Dir = prog
		// one cache for both modes: a whole program's entries and a
		// separate build's must not answer each other
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_SEPCOMP_BG=0"), env...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo build (%s): %v\n%s", strings.Join(env, " "), err, log)
		}
		return string(log)
	}
	run := func(bin string) string {
		t.Helper()
		out, err := testtmp.Guard(exec.Command(bin)).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", bin, err, out)
		}
		return string(out)
	}
	// std's variants in the test's cache first: a miss would build the
	// whole program (the Go tests run with XO_SEPCOMP_BG=0, so no
	// background build either)
	for _, c := range []string{"0", "1"} {
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc="+c))
		cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo dev stdcache --conc=%s: %v\n%s", c, err, out)
		}
	}
	whole := filepath.Join(work, "whole")
	build(whole, "XO_SEPCOMP=0")
	sep := filepath.Join(work, "sep")
	log := build(sep)
	if !strings.Contains(log, "cache std ") {
		t.Fatalf("the default build did not ask the std cache:\n%s", log)
	}
	if got, want := run(sep), run(whole); got != want || want != "3 C\n" {
		t.Fatalf("separately compiled %q, whole %q, want %q", got, want, "3 C\n")
	}
	// the whole program again from the shared cache: still whole
	whole2 := filepath.Join(work, "whole2")
	build(whole2, "XO_SEPCOMP=0")
	if run(whole2) != "3 C\n" {
		t.Fatal("the whole program rebuilt from the shared cache runs wrong")
	}
}

// TestSepcompSharedMade (decision 0388): a function the compiler makes on
// demand under a fixed name (xo.neterr for a network result, zero.T for
// a function type's zero value) is in std's store and in the program's
// part alike; both are weak, so the separately compiled link keeps one
// (Lang 1's default-on gate: `duplicate symbol _xo_f_xo_2eneterr` in the
// net goldens, `_xo_f_zero_2efn_28Int_...` in sql_driver), and the
// program prints its golden's output.
func TestSepcompSharedMade(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's variants and programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepmade-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=1"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	for _, g := range []string{"net_echo", "sql_driver"} {
		dir := filepath.Join("..", "llvm", "testdata", "golden", g)
		bin := filepath.Join(work, g)
		b := testtmp.Guard(exec.Command(xo, "build", "--backend=arm64", "-o", bin, "."))
		b.Dir = dir
		b.Env = append(os.Environ(), "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+lines[1], "XO_SEPCOMP_STORE="+lines[0], "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-"+g))
		if log, err := b.CombinedOutput(); err != nil {
			t.Fatalf("%s: xo build: %v\n%s", g, err, log)
		}
		run := testtmp.Guard(exec.Command("sh", filepath.Join("..", "..", "scripts", "golden-run.sh"), bin, dir))
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("%s: not the golden's output: %v\n%s", g, err, out)
		}
	}
}

// TestSepcompZeroValWeak: std's object defines its zero functions
// (zero.T) and fault exits' shared zero values (zero_val.T, ssa build.xo
// zero_val_of) weak, as a separately compiled program does: a program
// function built before any std function that makes one emits its own,
// and both copies were strong, so the link failed with `duplicate symbol
// _xo_f_zero__val_2enative_2ehttp_2eRequest` (aws issue 12, 2026-11-05:
// an S3 program's first build after std's variants were cached; a
// rebuild took std's copy). Which build makes it first depends on the
// program, so this reads std's part objects instead.
func TestSepcompZeroValWeak(t *testing.T) {
	t.Parallel()
	if !native() || runtime.GOOS != "darwin" {
		t.Skip("reads Mach-O objects with nm -m")
	}
	if testing.Short() {
		t.Skip("builds std's variant")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepzero-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	objs, err := filepath.Glob(filepath.Join(lines[0]+".parts", "*.o"))
	if err != nil || len(objs) == 0 {
		t.Fatalf("no part objects beside std's store %s: %v", lines[0], err)
	}
	seen := 0
	for _, o := range objs {
		out, err := exec.Command("nm", "-m", "--defined-only", o).Output()
		if err != nil {
			t.Fatalf("nm -m %s: %v", o, err)
		}
		for _, l := range strings.Split(string(out), "\n") {
			// the functions, not their local unwind tables ($ui, $ut)
			if !strings.Contains(l, "(__TEXT,__text)") || strings.Contains(l, "non-external") || (!strings.Contains(l, " _xo_f_zero__val_") && !strings.Contains(l, " _xo_f_zero_2e")) {
				continue
			}
			seen++
			if !strings.Contains(l, "weak") {
				t.Errorf("%s: a strong zero definition, which a program's own copy duplicates: %s", filepath.Base(o), strings.TrimSpace(l))
			}
		}
	}
	if seen == 0 {
		t.Fatalf("std's object defines no zero.T or zero_val.T function: the test no longer sees what it checks")
	}
}

// TestSepcompDebugInfo (decision 0388): a separately compiled program's
// debug information is a whole program's. Its first compile unit is its
// own (DW_AT_comp_dir the module's directory with --trimpath=off, as
// TestDebugLineDarwin wants: the store's std units came first, with
// "."), and -s or --debug-info=off leaves none, embedded or in a dSYM
// (std's store carries DWARF the program's own build left out).
func TestSepcompDebugInfo(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("dsymutil, dwarfdump and otool read a darwin binary's debug map (the linux/arm64 host, 2026-10-30)")
	}
	if testing.Short() {
		t.Skip("builds std's variants and programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepdbg-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"debug\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(out string, flags ...string) string {
		t.Helper()
		bin := filepath.Join(work, out)
		b := testtmp.Guard(exec.Command(xo, append(append([]string{"build", "--backend=arm64"}, flags...), "-o", bin, ".")...))
		b.Dir = prog
		b.Env = append(os.Environ(), "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+lines[1], "XO_SEPCOMP_STORE="+lines[0], "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-"+out))
		if log, err := b.CombinedOutput(); err != nil {
			t.Fatalf("xo build %s: %v\n%s", strings.Join(flags, " "), err, log)
		}
		return bin
	}
	// the DWARF through the binary's debug map (decision 0401: no __DWARF
	// in a separately compiled darwin binary): dsymutil's dSYM
	full := build("full", "--trimpath=off")
	if log, err := exec.Command("dsymutil", full, "-o", full+".dSYM").CombinedOutput(); err != nil {
		t.Fatalf("dsymutil: %v\n%s", err, log)
	}
	ci, err := exec.Command("dwarfdump", "--debug-info", full+".dSYM").CombinedOutput()
	if err != nil {
		t.Fatalf("dwarfdump: %v\n%s", err, ci)
	}
	cd := regexp.MustCompile(`DW_AT_comp_dir\s+\("([^"]*)"\)`).FindSubmatch(ci)
	want, _ := filepath.EvalSymlinks(prog)
	if cd == nil {
		t.Fatalf("no DW_AT_comp_dir in\n%s", ci)
	}
	if got, _ := filepath.EvalSymlinks(string(cd[1])); got != want {
		t.Errorf("the first unit's DW_AT_comp_dir is %s, want the program's %s", cd[1], prog)
	}
	// --debug-info=off: the program compiles none, and std's is left out
	if segs, _ := exec.Command("otool", "-l", build("off", "--debug-info=off")).CombinedOutput(); strings.Contains(string(segs), "segname __DWARF") || strings.Contains(string(segs), "segname __XODW") {
		t.Errorf("--debug-info=off left a __DWARF or __XODW segment")
	}
	stripped := build("q", "-s", "--dsym")
	if _, err := os.Stat(stripped + ".dSYM"); err == nil {
		t.Errorf("-s wrote %s.dSYM", stripped)
	}
	if segs, _ := exec.Command("otool", "-l", stripped).CombinedOutput(); strings.Contains(string(segs), "segname __DWARF") || strings.Contains(string(segs), "segname __XODW") {
		t.Errorf("-s left a __DWARF or __XODW segment in %s", stripped)
	}
}

// TestSepcompTestVariant (decision 0436): `xo test` links std's test
// variant (std built in test mode: Fs calls on the test world's MemFs)
// when it is in the cache, and its tests, including one whose std
// calls go through MemFs, pass as the whole program's do.
func TestSepcompTestVariant(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's test variants")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-septest-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "fn main(os: Os) {\n}\n\ntest \"fs\" {\n  os.fs.create_dir_all(\"d/e\").or_fault(\"mk\")\n  os.fs.write(\"d/e/x.txt\", b\"hi\").or_fault(\"w\")\n  os.fs.copy(\"d/e/x.txt\", \"d/y.txt\").or_fault(\"cp\")\n  expect(os.fs.read(\"d/y.txt\").or_fault(\"r\")) == b\"hi\"\n  expect(os.fs.glob(\"d/*.txt\").or_fault(\"g\").len()) == 1\n  os.fs.remove_all(\"d\").or_fault(\"rm\")\n  expect(os.fs.exists(\"d\").or_fault(\"ex\")) == false\n}\n"
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BIN_CACHE="+filepath.Join(work, "bin"), "XO_SEPCOMP_BG=0")
	for _, c := range []string{"0", "1"} {
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--test", "--conc="+c))
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo dev stdcache --test --conc=%s: %v\n%s", c, err, out)
		}
	}
	test := func(extra ...string) string {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "test", "--backend=arm64", "--phase-times", "."))
		cmd.Dir = prog
		cmd.Env = append(append([]string{}, env...), extra...)
		log, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xo test (%s): %v\n%s", strings.Join(extra, " "), err, log)
		}
		return string(log)
	}
	whole := test("XO_SEPCOMP=0")
	if !strings.Contains(whole, "1 passed, 0 failed") {
		t.Fatalf("whole program:\n%s", whole)
	}
	os.RemoveAll(filepath.Join(work, "bin", "run"))
	sep := test()
	if !strings.Contains(sep, "test=1 reused") || !strings.Contains(sep, "1 passed, 0 failed") {
		t.Fatalf("with std's test variant:\n%s", sep)
	}
}

// TestSepcompStdModuleEdits (item 2 of the test speed queue): `xo test
// --std-module` links std's test variant, which is keyed without std's
// _test.xo files, so a module's edited tests still take the variant and
// are compiled fresh (one more test passes), and an edit to the module's
// own source never runs the store's stale code (the probe sees the new
// body).
func TestSepcompStdModuleEdits(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds std's test variants")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepstdmod-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	// a copy of std to edit
	std := filepath.Join(work, "std")
	if out, err := exec.Command("cp", "-R", filepath.Join(root, "std"), std).CombinedOutput(); err != nil {
		t.Fatalf("copy std: %v\n%s", err, out)
	}
	wd := filepath.Join(work, "wd")
	if err := os.MkdirAll(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "XO_STD="+std, "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "cache"), "XO_BIN_CACHE="+filepath.Join(work, "bin"), "XO_SEPCOMP_BG=0")
	for _, c := range []string{"0", "1"} {
		cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--test", "--conc="+c))
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo dev stdcache --test --conc=%s: %v\n%s", c, err, out)
		}
	}
	run := func() (string, error) {
		t.Helper()
		cmd := testtmp.Guard(exec.Command(xo, "test", "--backend=arm64", "--phase-times", "--std-module=native/base64"))
		cmd.Dir = wd
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	passed := func(log string) int {
		m := regexp.MustCompile(`(\d+) passed`).FindStringSubmatch(log)
		if m == nil {
			t.Fatalf("no summary:\n%s", log)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	appendTo := func(p, s string) {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	log, err := run()
	if err != nil || !strings.Contains(log, "test=1 reused") {
		t.Fatalf("unchanged std: %v\n%s", err, log)
	}
	n := passed(log)
	tests := filepath.Join(std, "native", "base64_test.xo")
	appendTo(tests, "\ntest \"sepcomp edit\" {\n  expect(encode(b\"x\")) == \"eA==\"\n}\n")
	log, err = run()
	if err != nil || !strings.Contains(log, "test=1 reused") || passed(log) != n+1 {
		t.Fatalf("after a test edit, want the variant reused and %d passed: %v\n%s", n+1, err, log)
	}
	appendTo(filepath.Join(std, "native", "base64.xo"), "\nfn sepcomp_probe() -> Int { 41 }\n")
	appendTo(tests, "\ntest \"sepcomp probe\" {\n  expect(sepcomp_probe()) == 42\n}\n")
	log, err = run()
	if err == nil || !strings.Contains(log, "sepcomp probe") {
		t.Fatalf("after a source edit the probe (41, want 42) must fail by name:\n%s", log)
	}
	// the rest of std from the variant, base64 and its importers from
	// source
	if !strings.Contains(log, "module(s) from source") {
		t.Fatalf("after a source edit, want the variant reused with base64 from source:\n%s", log)
	}
	src := filepath.Join(std, "native", "base64.xo")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(strings.Replace(string(b), "{ 41 }", "{ 42 }", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err = run()
	if err != nil || !strings.Contains(log, "module(s) from source") || passed(log) != n+2 {
		t.Fatalf("after fixing the probe, want the variant reused and %d passed: %v\n%s", n+2, err, log)
	}
}
