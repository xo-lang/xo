package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
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
// of its own, and calls std's list, string and sort functions.
func TestSepcompLinks(t *testing.T) {
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
}

// TestSepcompDefault (decision 0388 stage 3, step d): with the default on
// (XO_SEPCOMP_DEFAULT=1 until it is flipped), a build for the host takes
// std's variants from the std cache and links separately (its phase line
// says so), runs as the whole program (XO_SEPCOMP=0) does, and
// XO_SEPCOMP=0 is never answered from the separately compiled entries.
func TestSepcompDefault(t *testing.T) {
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
	log := build(sep, "XO_SEPCOMP_DEFAULT=1")
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
