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
