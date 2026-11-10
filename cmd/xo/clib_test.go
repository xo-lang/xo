package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// clibSrc is a C library (decision 0574 stage 1): Int, Float, Bytes and
// Str parameters, a List on the heap xo_init made.
const clibSrc = `/// The sum of a and b.
pub fn add(a: Int, b: Int) -> Int as c "geo_add" {
  a + b
}

/// The length of the vector (x, y).
pub fn norm(x: Float, y: Float) -> Float as c "geo_norm" {
  (x * x + y * y).sqrt()
}

/// The sum of 0 to n - 1 through a List.
pub fn list_sum(n: Int) -> Int as c "geo_list_sum" {
  var xs: List[Int] = []
  for i in 0..n {
    xs.push(i)
  }
  var s = 0
  for x in xs {
    s += x
  }
  s
}

/// The sum of the bytes of b.
pub fn byte_sum(b: Bytes) -> Int as c "geo_byte_sum" {
  var s = 0
  for x in b {
    s += Int(x)
  }
  s
}

/// w times the length of s plus k: a Str's two words between a Float
/// and an Int.
pub fn str_len(w: Float, s: Str, k: Int) -> Float as c "geo_str_len" {
  w * Float(s.len() + k)
}
`

const clibCaller = `#include <math.h>
#include <stdio.h>
#include "geo.h"

int main(int argc, char **argv, char **envp) {
  if (xo_init(argc, argv, envp) != 0) { puts("xo_init"); return 1; }
  if (geo_add(2, 40) != 42) { puts("geo_add"); return 1; }
  if (fabs(geo_norm(3.0, 4.0) - 5.0) > 1e-12) { puts("geo_norm"); return 1; }
  if (geo_list_sum(100000) != 4999950000LL) { puts("geo_list_sum"); return 1; }
  uint8_t buf[64];
  int64_t want = 0;
  for (int i = 0; i < 64; i++) { buf[i] = (uint8_t)(i * 7); want += buf[i]; }
  XoBytes b = {buf, 64};
  if (geo_byte_sum(b) != want) { puts("geo_byte_sum"); return 1; }
  XoBytes none = {NULL, 0};
  if (geo_byte_sum(none) != 0) { puts("geo_byte_sum empty"); return 1; }
  XoStr s = {"hello", 5};
  if (geo_str_len(0.5, s, 3) != 4.0) { puts("geo_str_len"); return 1; }
  /* invalid UTF-8: the 0xff byte arrives as U+FFFD, three bytes */
  XoStr bad = {"a\xff" "b", 3};
  if (geo_str_len(1.0, bad, 0) != 5.0) { puts("geo_str_len utf8"); return 1; }
  puts("ok");
  return 0;
}
`

// clibTail is the last lines of a failed build's output on one line,
// so a failure stays self-describing in a log filtered to its first line
// (a TestCLibThreadExit failure on 2026-11-08 lost its cause that way).
func clibTail(s string) string {
	var ls []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "hint") {
			ls = append(ls, l)
		}
	}
	if len(ls) == 0 {
		return "(no output)"
	}
	if len(ls) > 4 {
		ls = ls[len(ls)-4:]
	}
	return strings.Join(ls, " | ")
}

// clibSkip skips where --lib does not build: native arm64 and amd64, on
// darwin and linux.
func clibSkip(t *testing.T) {
	t.Helper()
	if (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		t.Skip("--lib builds libraries on darwin and linux, arm64 and amd64")
	}
}

// TestCLibShared builds a C library with `xo build --lib=shared`
// (decision 0574) and calls it from C through the
// generated header: Int, Float, Bytes and Str arguments, and the heap.
func TestCLibShared(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/geo\nedition preview\n",
		"geo.xo": clibSrc,
		"main.c": clibCaller,
	})
	lib := filepath.Join(dir, "libgeo.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libgeo.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	h, err := os.ReadFile(filepath.Join(dir, "geo.h"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"int64_t xo_init(int argc, char **argv, char **envp);", "int64_t geo_add(int64_t a, int64_t b);", "int64_t geo_byte_sum(XoBytes b);", "double geo_str_len(double w, XoStr s, int64_t k);", "typedef struct XoStr { const char *p; int64_t n; } XoStr;"} {
		if !strings.Contains(string(h), want) {
			t.Errorf("geo.h has no %q:\n%s", want, h)
		}
	}
	// only xo_init and the exports are the library's symbols
	nmArgs := []string{"-gU", lib}
	if runtime.GOOS == "linux" {
		nmArgs = []string{"-D", "--defined-only", lib}
	}
	if b, err := exec.Command("nm", nmArgs...).CombinedOutput(); err == nil {
		var syms []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			f := strings.Fields(l)
			if len(f) == 3 && f[1] == "T" {
				syms = append(syms, strings.TrimPrefix(f[2], "_"))
			}
		}
		if got := strings.Join(syms, " "); got != "geo_add geo_byte_sum geo_list_sum geo_norm geo_str_len xo_free xo_init xo_last_error xo_release" {
			t.Errorf("the library's symbols: %s", got)
		}
	}
	exe := filepath.Join(dir, "geotest")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "main.c"), "-L"+dir, "-lgeo", "-Wl,-rpath,"+dir, "-lm").CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("the C caller: %v\n%s", err, b)
	}
}

// TestCLibStatic builds the same library with `xo build --lib=static`
// and links it into the C caller (decision 0574 stage 1), and into one
// that never calls xo_init (the archive's constructor starts it).
func TestCLibStatic(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/geo\nedition preview\n",
		"geo.xo": clibSrc,
		"main.c": clibCaller,
	})
	lib := filepath.Join(dir, "libgeo.a")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=static", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=static: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "geotest")
	args := []string{"-O2", "-I" + dir, "-o", exe, filepath.Join(dir, "main.c"), lib, "-lm"}
	if runtime.GOOS == "linux" {
		args = append(args, "-lpthread", "-ldl")
	}
	if b, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("the C caller: %v\n%s", err, b)
	}
	noinit := filepath.Join(dir, "noinit.c")
	if err := os.WriteFile(noinit, []byte("#include <stdio.h>\n#include \"geo.h\"\nint main(void) {\n  if (geo_list_sum(1000) != 499500) { puts(\"sum\"); return 1; }\n  puts(\"ok\");\n  return 0;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe2 := filepath.Join(dir, "noinit")
	args2 := append([]string{"-O2", "-I" + dir, "-o", exe2, noinit}, args[5:]...)
	if b, err := exec.Command(cc, args2...).CombinedOutput(); err != nil {
		t.Fatalf("cc (no xo_init): %v\n%s", err, b)
	}
	b, err = exec.Command(exe2).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("the C caller without xo_init: %v\n%s", err, b)
	}
	// a constructor of the program's, which runs before the archive's
	// (link order; on ELF its priority too), calls an export first
	early := filepath.Join(dir, "early.c")
	if err := os.WriteFile(early, []byte("#include <stdio.h>\n#include \"geo.h\"\nstatic int64_t first;\n__attribute__((constructor(101))) static void before(void) { first = geo_list_sum(1000); }\nint main(void) {\n  if (first != 499500 || geo_list_sum(1000) != 499500) { puts(\"sum\"); return 1; }\n  puts(\"ok\");\n  return 0;\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe3 := filepath.Join(dir, "early")
	args3 := append([]string{"-O2", "-I" + dir, "-o", exe3, early}, args[5:]...)
	if b, err := exec.Command(cc, args3...).CombinedOutput(); err != nil {
		t.Fatalf("cc (early constructor): %v\n%s", err, b)
	}
	b, err = exec.Command(exe3).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("an export called from an earlier constructor: %v\n%s", err, b)
	}
}

// TestCLibRejects covers XO1518 (an `as c` function C cannot call) and a
// library with `fn main`.
func TestCLibRejects(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"private", "fn f(a: Int) -> Int as c \"f\" { a }\n", "it must be `pub fn`"},
		{"reserved", "pub fn f(a: Int) -> Int as c \"xo_f\" { a }\n", "the `xo_` names are the runtime's"},
		{"ident", "pub fn f(a: Int) -> Int as c \"f-g\" { a }\n", "is not a C identifier"},
		{"bool", "pub fn f(a: Bool) -> Int as c \"f\" { 0 }\n", "(not Bool or a function)"},
		{"func", "pub fn f(a: fn(Int) -> Int) -> Int as c \"f\" { 0 }\n", "(not Bool or a function)"},
		{"result", "pub fn f(a: Int) -> Bool as c \"f\" { true }\n", "a C export returns Int, Float, Bytes, Str, a value held by XoHandle, or nothing"},
		{"words", "pub fn f(a: Bytes, b: Bytes, c: Bytes, d: Bytes, e: Int) -> Int as c \"f\" { 0 }\n", "need 9 integer registers"},
		{"generic", "pub fn f[T](a: T) -> Int as c \"f\" { 0 }\n", "a generic function cannot be one"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := writeFiles(t, map[string]string{
				"xo.mod": "module example.com/lib\nedition preview\n",
				"lib.xo": c.src,
			})
			var out, errOut bytes.Buffer
			code := run(t, []string{"check", dir}, &out, &errOut)
			got := out.String() + errOut.String()
			if code == 0 || !strings.Contains(got, "XO1518") || !strings.Contains(got, c.want) {
				t.Fatalf("xo check: exit %d, want XO1518 with %q:\n%s", code, c.want, got)
			}
		})
	}
	if (runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64") || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		return
	}
	t.Run("main", func(t *testing.T) {
		t.Parallel()
		dir := writeFiles(t, map[string]string{
			"xo.mod":  "module example.com/lib\nedition preview\n",
			"main.xo": "pub fn add(a: Int, b: Int) -> Int as c \"lib_add\" { a + b }\n\nfn main() {}\n",
		})
		var out, errOut bytes.Buffer
		code := run(t, []string{"build", "--lib=shared", "-o", filepath.Join(dir, "liblib.so"), dir}, &out, &errOut)
		if code == 0 {
			t.Fatalf("xo build --lib=shared of a package with fn main succeeded:\n%s%s", out.String(), errOut.String())
		}
	})
}

const clibThreads = `#include <pthread.h>
#include <stdio.h>
#include "geo.h"

static void *run(void *arg) {
  long bad = 0;
  for (int i = 0; i < 20000; i++) if (geo_list_sum(500) != 124750) bad++;
  *(long *)arg = bad;
  return NULL;
}

int main(int argc, char **argv, char **envp) {
  if (xo_init(argc, argv, envp) != 0) { puts("xo_init"); return 1; }
  pthread_t t[4];
  long bad[4];
  for (int i = 0; i < 4; i++) pthread_create(&t[i], NULL, run, &bad[i]);
  for (int i = 0; i < 4; i++) pthread_join(t[i], NULL);
  if (bad[0] + bad[1] + bad[2] + bad[3] != 0) { puts("wrong sums"); return 1; }
  puts("ok");
  return 0;
}
`

// TestCLibThreads calls an allocating export from four C threads at once
// (decision 0574: xo_init gives each thread its own heap record; the
// single threaded heap crashed).
func TestCLibThreads(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":    "module example.com/geo\nedition preview\n",
		"geo.xo":    clibSrc,
		"threads.c": clibThreads,
	})
	lib := filepath.Join(dir, "libgeo.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libgeo.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "threads")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "threads.c"), "-L"+dir, "-lgeo", "-Wl,-rpath,"+dir, "-lpthread").CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("four C threads: %v\n%s", err, b)
	}
}

// TestCLibFault: a fault in an export prints the fault report and exits
// 70 (decision 0574 stage 1); the traceback names the export's function
// from the library's symbols and ends at its C caller.
func TestCLibFault(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/boom\nedition preview\n",
		"boom.xo": "/// Element n of a two element list.\npub fn boom(n: Int) -> Int as c \"boom_at\" {\n  let xs = [10, 20]\n  xs[n]\n}\n",
		"main.c":  "#include <stdio.h>\n#include \"boom.h\"\nint main(int argc, char **argv, char **envp) {\n  xo_init(argc, argv, envp);\n  printf(\"%lld\\n\", (long long)boom_at(5));\n  return 0;\n}\n",
	})
	lib := filepath.Join(dir, "libboom.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libboom.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "boom")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "main.c"), "-L"+dir, "-lboom", "-Wl,-rpath,"+dir).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 70 || !strings.Contains(string(b), "fault: index 5 out of range for List of length 2 at boom.xo:4:3") {
		t.Fatalf("want the fault report and exit 70, got %v:\n%s", err, b)
	}
	// the C caller: its function on darwin (dladdr), its file on linux
	caller := "main (C)"
	if runtime.GOOS == "linux" {
		caller = "[boom] (C)"
	}
	if !strings.Contains(string(b), "traceback:\n  boom.boom\n  "+caller+"\n") {
		t.Errorf("the traceback does not name boom.boom and its C caller:\n%s", b)
	}
}

// TestCLibNoInit calls a shared library's exports without xo_init: the
// library's constructor started the runtime when it was loaded, and a
// later xo_init does nothing (decision 0574).
func TestCLibNoInit(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/geo\nedition preview\n",
		"geo.xo": clibSrc,
		"main.c": "#include <stdio.h>\n#include \"geo.h\"\nint main(int argc, char **argv, char **envp) {\n  if (geo_list_sum(1000) != 499500) { puts(\"before\"); return 1; }\n  if (xo_init(argc, argv, envp) != 0) { puts(\"xo_init\"); return 1; }\n  if (geo_list_sum(1000) != 499500) { puts(\"after\"); return 1; }\n  puts(\"ok\");\n  return 0;\n}\n",
	})
	// a library is built whole even when separate compilation is asked
	// for: std's variants are a program's, without xo_init's callees (a
	// cached variant made every --lib link fail, 2026-11-08)
	setenv(t, "XO_SEPCOMP", "1")
	setenv(t, "XO_SEPCOMP_FACTS", filepath.Join(dir, "no-such-facts"))
	lib := filepath.Join(dir, "libgeo.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libgeo.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "noinit")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "main.c"), "-L"+dir, "-lgeo", "-Wl,-rpath,"+dir).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("exports without xo_init: %v\n%s", err, b)
	}
}

// TestCLibErrors calls exports with error results from C: the value and
// err 0 on success, err 1 and the text in xo_last_error on failure, and a
// NULL err.
func TestCLibErrors(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/calc\nedition preview\n",
		"calc.xo": "pub fn div(a: Int, b: Int) -> Int ! Str as c \"calc_div\" {\n  if b == 0 {\n    return Err(\"division by zero\")\n  }\n  a / b\n}\n\n" +
			"pub fn half(x: Float) -> Float ! Str as c \"calc_half\" {\n  if x < 0.0 {\n    return Err(\"negative: ${x}\")\n  }\n  x / 2.0\n}\n\n" +
			"pub fn check(s: Str) -> () ! Str as c \"calc_check\" {\n  if s.is_empty() {\n    return Err(\"empty\")\n  }\n}\n",
		"main.c": `#include <stdio.h>
#include <string.h>
#include "calc.h"
int main(void) {
  int32_t err = 7;
  if (calc_div(7, 2, &err) != 3 || err != 0) { puts("div ok"); return 1; }
  if (xo_last_error() != NULL) { puts("no error yet"); return 1; }
  if (calc_div(1, 0, &err) != 0 || err != 1 || strcmp(xo_last_error(), "division by zero") != 0) { puts("div err"); return 1; }
  if (calc_div(1, 0, NULL) != 0) { puts("div null"); return 1; }
  if (calc_half(3.0, &err) != 1.5 || err != 0) { puts("half ok"); return 1; }
  if (calc_half(-1.5, &err) != 0.0 || err != 1 || strcmp(xo_last_error(), "negative: -1.5") != 0) { printf("half err %s\n", xo_last_error()); return 1; }
  XoStr s = {"x", 1}, e = {"", 0};
  calc_check(s, &err);
  if (err != 0) { puts("check ok"); return 1; }
  calc_check(e, &err);
  if (err != 1 || strcmp(xo_last_error(), "empty") != 0) { puts("check err"); return 1; }
  puts("ok");
  return 0;
}
`,
	})
	lib := filepath.Join(dir, "libcalc.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libcalc.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	h, err := os.ReadFile(filepath.Join(dir, "calc.h"))
	if err != nil || !strings.Contains(string(h), "int64_t calc_div(int64_t a, int64_t b, int32_t *err);") || !strings.Contains(string(h), "void calc_check(XoStr s, int32_t *err);") {
		t.Fatalf("calc.h: %v\n%s", err, h)
	}
	exe := filepath.Join(dir, "calc")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "main.c"), "-L"+dir, "-lcalc", "-Wl,-rpath,"+dir).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("error results: %v\n%s", err, b)
	}
}

// clibHandles is a library whose exports pass values C cannot hold as
// XoHandles (decision 0574 stage 2).
const clibHandles = `pub struct Counter {
  name: Str
  n: Int
}

pub fn counter(name: Str) -> Counter as c "ctr_new" { Counter{name: name, n: 0} }

pub fn add(c: Counter, k: Int) -> Counter as c "ctr_add" { Counter{name: c.name, n: c.n + k} }

pub fn count(c: Counter) -> Int as c "ctr_count" { c.n }

pub fn name_len(c: Counter) -> Int as c "ctr_name_len" { c.name.len() }

pub fn squares(n: Int) -> List[Int] as c "ctr_squares" {
  var xs: List[Int] = []
  for i in 0..n {
    xs.push(i * i)
  }
  xs
}

pub fn total(xs: List[Int]) -> Int as c "ctr_total" {
  var t = 0
  for x in xs {
    t += x
  }
  t
}

pub fn above(c: Counter, min: Int) -> Option[Counter] as c "ctr_above" {
  if c.n > min { Some(c) } else { None }
}

pub fn count_or(c: Option[Counter], dflt: Int) -> Int as c "ctr_count_or" {
  match c {
    Some(x) => x.n
    None => dflt
  }
}

pub fn maybe_n(c: Counter) -> Option[Int] as c "ctr_maybe_n" { Some(c.n) }

pub struct Held {
  b: Bytes
}

pub fn hold(b: Bytes) -> Held as c "ctr_hold" { Held{b: b} }

pub fn hold_tail(b: Bytes) -> Held as c "ctr_hold_tail" { Held{b: b[1..b.len()]} }

pub fn held_sum(h: Held) -> Int as c "ctr_held_sum" {
  var t = 0
  for x in h.b {
    t += Int(x)
  }
  t
}

pub fn both_len(a: Bytes, s: Str) -> Int as c "ctr_both_len" { a.len() * 100 + s.len() }

pub fn label(c: Counter) -> Str as c "ctr_label" { "${c.name}=${c.n}" }

pub fn tail(b: Bytes, k: Int) -> Bytes as c "ctr_tail" { b[k..b.len()] }

pub fn label_or(c: Counter, ok: Int) -> Str ! Str as c "ctr_label_or" {
  if ok == 0 {
    return Err("no label")
  }
  "${c.name}"
}
`

// TestCLibHandles passes a struct and a List through XoHandles: made by
// one export, read and replaced by others, released; then a released
// handle and a handle of another type each fault (exit 70).
func TestCLibHandles(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod": "module example.com/ctr\nedition preview\n",
		"ctr.xo": clibHandles,
		"main.c": `#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "ctr.h"
int main(int argc, char **argv) {
  XoStr nm = {"hits", 4};
  XoHandle c = ctr_new(nm);
  XoHandle d = ctr_add(c, 5);
  XoHandle e = ctr_add(d, 7);
  if (ctr_count(c) != 0 || ctr_count(d) != 5 || ctr_count(e) != 12 || ctr_name_len(e) != 4) { puts("counter"); return 1; }
  xo_release(c);
  xo_release(d);
  if (ctr_count(e) != 12) { puts("after release"); return 1; }
  long sum = 0;
  for (int i = 0; i < 10000; i++) {
    XoHandle xs = ctr_squares(10);
    sum += ctr_total(xs);
    xo_release(xs);
  }
  if (sum != 2850000) { printf("squares %ld\n", sum); return 1; }
  XoHandle a = ctr_above(e, 10), n = ctr_above(e, 12);
  if (a == 0 || n != 0 || ctr_count(a) != 12 || ctr_count_or(a, -1) != 12 || ctr_count_or(0, -1) != -1) { puts("option"); return 1; }
  xo_release(a);
  for (int i = 0; i < 10000; i++) {
    XoStr l = ctr_label(e);
    if (l.n != 7 || memcmp(l.p, "hits=12", 7) != 0 || l.p[7] != 0) { printf("label %lld %s\n", (long long)l.n, l.p); return 1; }
    xo_free(l.p);
  }
  XoBytes in = {(const uint8_t *)"abcdef", 6};
  XoBytes tl = ctr_tail(in, 2);
  if (tl.n != 4 || memcmp(tl.p, "cdef", 4) != 0) { puts("tail"); return 1; }
  xo_free(tl.p);
  xo_free(NULL);
  int32_t err = 7;
  XoStr lo = ctr_label_or(e, 1, &err);
  if (err != 0 || lo.n != 4 || memcmp(lo.p, "hits", 4) != 0) { puts("label_or"); return 1; }
  xo_free(lo.p);
  lo = ctr_label_or(e, 0, &err);
  if (err != 1 || lo.p != NULL || lo.n != 0 || strcmp(xo_last_error(), "no label") != 0) { puts("label_or error"); return 1; }
  /* a Bytes the function keeps, whole or as a view, is never a later call's buffer */
  XoBytes abc = {(const uint8_t *)"abc", 3}, xyz = {(const uint8_t *)"xyzw", 4};
  XoHandle h1 = ctr_hold(abc), h2 = ctr_hold_tail(abc);
  for (int i = 0; i < 1000; i++) {
    XoStr st = {"hello", 5};
    if (ctr_both_len(xyz, st) != 405) { puts("both_len"); return 1; }
    xo_release(ctr_hold(xyz));
  }
  if (ctr_held_sum(h1) != 'a' + 'b' + 'c' || ctr_held_sum(h2) != 'b' + 'c') { printf("held %lld %lld\n", (long long)ctr_held_sum(h1), (long long)ctr_held_sum(h2)); return 1; }
  xo_release(h1);
  xo_release(h2);
  XoHandle m = ctr_maybe_n(e);
  if (m == 0) { puts("option int"); return 1; }
  xo_release(m);
  xo_release(0);
  if (argc > 1 && strcmp(argv[1], "stale") == 0) { ctr_count(c); puts("no fault"); return 1; }
  if (argc > 1 && strcmp(argv[1], "type") == 0) { XoHandle xs = ctr_squares(3); ctr_count(xs); puts("no fault"); return 1; }
  xo_release(e);
  puts("ok");
  return 0;
}
`,
	})
	lib := filepath.Join(dir, "libctr.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libctr.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	h, err := os.ReadFile(filepath.Join(dir, "ctr.h"))
	if err != nil || !strings.Contains(string(h), "XoHandle /* Counter */ ctr_add(XoHandle /* Counter */ c, int64_t k);") || !strings.Contains(string(h), "void xo_release(XoHandle h);") || !strings.Contains(string(h), "XoHandle /* Counter or 0 */ ctr_above(XoHandle /* Counter */ c, int64_t min);") || !strings.Contains(string(h), "int64_t ctr_count_or(XoHandle /* Counter or 0 */ c, int64_t dflt);") || !strings.Contains(string(h), "XoHandle /* Int? */ ctr_maybe_n(") || !strings.Contains(string(h), "XoStr ctr_label(XoHandle /* Counter */ c);") || !strings.Contains(string(h), "XoBytes ctr_tail(XoBytes b, int64_t k);") || !strings.Contains(string(h), "void xo_free(const void *p);") {
		t.Fatalf("ctr.h: %v\n%s", err, h)
	}
	exe := filepath.Join(dir, "ctr")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "main.c"), "-L"+dir, "-lctr", "-Wl,-rpath,"+dir).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("handles: %v\n%s", err, b)
	}
	for _, c := range []struct{ arg, want string }{
		{"stale", "is not live"},
		{"type", "holds a value of another type"},
	} {
		b, err := exec.Command(exe, c.arg).CombinedOutput()
		ee, ok := err.(*exec.ExitError)
		if !ok || ee.ExitCode() != 70 || !strings.Contains(string(b), c.want) {
			t.Errorf("%s handle: want a fault naming %q and exit 70, got %v:\n%s", c.arg, c.want, err, b)
		}
	}
}

// clibHandleChurn reads one long lived handle from 4 C threads while 4
// others make, read and release their own (decision 0574 stage 2: a
// handle parameter is read in place inside the table's read epoch, and a
// released value waits until the reads that began before its release
// end).
const clibHandleChurn = `#include <pthread.h>
#include <stdio.h>
#include "ctr.h"

static XoHandle shared;
static volatile int bad;

static void *reader(void *arg) {
  (void)arg;
  for (int i = 0; i < 200000; i++) if (ctr_count(shared) != 7 || ctr_name_len(shared) != 6) bad++;
  return NULL;
}

static void *churn(void *arg) {
  long k = (long)arg;
  XoStr nm = {"churn", 5};
  XoHandle base = ctr_new(nm);
  for (int i = 0; i < 50000; i++) {
    XoHandle d = ctr_add(base, k + i);
    if (ctr_count(d) != k + i || ctr_name_len(d) != 5) bad++;
    xo_release(d);
  }
  xo_release(base);
  return NULL;
}

int main(void) {
  XoStr nm = {"shared", 6};
  XoHandle z = ctr_new(nm);
  shared = ctr_add(z, 7);
  xo_release(z);
  pthread_t t[8];
  for (long i = 0; i < 4; i++) pthread_create(&t[i], NULL, reader, NULL);
  for (long i = 0; i < 4; i++) pthread_create(&t[4 + i], NULL, churn, (void *)(i * 1000000));
  for (int i = 0; i < 8; i++) pthread_join(t[i], NULL);
  xo_release(shared);
  if (bad) { printf("bad %d\n", bad); return 1; }
  puts("ok");
  return 0;
}
`

// TestCLibHandleChurn runs clibHandleChurn: no read sees a wrong value
// or a freed one while other threads release.
func TestCLibHandleChurn(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module example.com/ctr\nedition preview\n",
		"ctr.xo":  clibHandles,
		"churn.c": clibHandleChurn,
	})
	lib := filepath.Join(dir, "libctr.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libctr.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "churn")
	if b, err := exec.Command(cc, "-O2", "-pthread", "-I"+dir, "-o", exe, filepath.Join(dir, "churn.c"), "-L"+dir, "-lctr", "-Wl,-rpath,"+dir).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("handle churn: %v\n%s", err, b)
	}
}

const clibThreadExit = `#include <pthread.h>
#include <stdio.h>
#include <sys/resource.h>
#include "geo.h"

static void *run(void *arg) { (void)arg; geo_list_sum(100); return NULL; }

static long rss_kb(void) {
  struct rusage u;
  getrusage(RUSAGE_SELF, &u);
#ifdef __APPLE__
  return u.ru_maxrss / 1024;
#else
  return u.ru_maxrss;
#endif
}

int main(void) {
  long before = 0;
  for (int i = 0; i < 500; i++) {
    if (i == 20) before = rss_kb();
    pthread_t t;
    pthread_create(&t, NULL, run, NULL);
    pthread_join(t, NULL);
  }
  long grew = rss_kb() - before;
  if (grew > 2048) { printf("max RSS grew %ld KB over 480 exited threads\n", grew); return 1; }
  puts("ok");
  return 0;
}
`

// TestCLibThreadExit runs 500 C threads that each call an allocating
// export and exit: each exited thread's heap record goes back for the
// next (decision 0574), where each kept about 32 KB before.
func TestCLibThreadExit(t *testing.T) {
	t.Parallel()
	clibSkip(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":   "module example.com/geo\nedition preview\n",
		"geo.xo":   clibSrc,
		"thexit.c": clibThreadExit,
	})
	lib := filepath.Join(dir, "libgeo.so")
	if runtime.GOOS == "darwin" {
		lib = filepath.Join(dir, "libgeo.dylib")
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"build", "--lib=shared", "-o", lib, dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo build --lib=shared: exit %d: %s\n%s%s", code, clibTail(out.String()+errOut.String()), out.String(), errOut.String())
	}
	exe := filepath.Join(dir, "thexit")
	if b, err := exec.Command(cc, "-O2", "-I"+dir, "-o", exe, filepath.Join(dir, "thexit.c"), "-L"+dir, "-lgeo", "-Wl,-rpath,"+dir, "-lpthread").CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, b)
	}
	b, err := exec.Command(exe).CombinedOutput()
	if err != nil || strings.TrimSpace(string(b)) != "ok" {
		t.Fatalf("exited C threads: %v\n%s", err, b)
	}
}
