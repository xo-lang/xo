package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestDepLazyBodies (decision 0517, on by default; XO_DEP_LAZY=0 is the
// eager check): a build checks a
// dependency's function bodies as it reaches them. A type error in a body
// the program never reaches no longer fails the build; one in a reached
// body still does; `xo check` of the dependency still reports both; and a
// dependency's non-pub Display and Debug methods, which another module
// calls (decision 0496 (c)) and whose effects are inferred, are checked
// before their callers read those effects.
func TestDepLazyBodies(t *testing.T) {
	ws := t.TempDir()
	lib := "pub struct Pt {\n  pub x: Int\n  pub y: Int\n}\n\nfn (p: Pt) display() -> Str {\n  \"(${p.x}, ${p.y})\"\n}\n\nfn (p: Pt) debug() -> Str {\n  \"Pt{x: ${p.x}}\"\n}\n\npub fn seven() -> Int {\n  7\n}\n\npub fn unused() -> Int {\n  \"not an Int\"\n}\n"
	modtest.WriteFiles(t, ws, map[string]string{
		"xo.work":          "xo preview\nuse ./app\nuse ./lib\n",
		"app/xo.mod":       "module example.com/app\nedition preview\n",
		"app/main/main.xo": "use example.com/lib/shapes\n\nfn show(d: Display) -> Str {\n  d.display()\n}\n\nfn main(os: Os) {\n  let p = shapes.Pt{x: 1, y: 2}\n  os.stdio.println(\"${p.display()} ${p.debug()} ${show(p)} ${shapes.seven()}\")\n}\n",
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/shapes/s.xo":  lib,
	})
	setenv(t, "XO_WORK", filepath.Join(ws, "xo.work"))
	main := filepath.Join(ws, "app", "main")
	// eager (XO_DEP_LAZY=0): the unreached body's error fails the build
	setenv(t, "XO_DEP_LAZY", "0")
	xo(t, "build", "-o", filepath.Join(ws, "bin"), main).want(1)
	setenv(t, "XO_DEP_LAZY", "")
	r := xo(t, "run", main).want(0)
	if want := "(1, 2) Pt{x: 1} (1, 2) 7\n"; r.out != want {
		t.Errorf("run with XO_DEP_LAZY=1: got %q, want %q\n%s", r.out, want, r.err)
	}
	// the dependency's own check stays complete
	r = xo(t, "check", "--json", filepath.Join(ws, "lib", "shapes")).want(1)
	if !strings.Contains(r.out, "s.xo") {
		t.Errorf("check of the dependency: the unreached body's error is missing:\n%s", r.out)
	}
	// a reached body's error still fails the build
	modtest.WriteFiles(t, ws, map[string]string{"lib/shapes/s.xo": strings.Replace(lib, "pub fn seven() -> Int {\n  7\n}", "pub fn seven() -> Int {\n  \"seven\"\n}", 1)})
	r = xo(t, "build", "-o", filepath.Join(ws, "bin"), main).want(1)
	if !strings.Contains(r.out+r.err, "s.xo") {
		t.Errorf("build with a reached body's error: the error is missing:\n%s%s", r.out, r.err)
	}
	// so does one in a non-pub Display method another module calls
	modtest.WriteFiles(t, ws, map[string]string{"lib/shapes/s.xo": strings.Replace(lib, "\"(${p.x}, ${p.y})\"", "p.x", 1)})
	r = xo(t, "build", "-o", filepath.Join(ws, "bin"), main).want(1)
	if !strings.Contains(r.out+r.err, "s.xo") {
		t.Errorf("build with a display method's error: the error is missing:\n%s%s", r.out, r.err)
	}
	// a non-pub Display method's inferred effect (it logs) reaches a pub
	// caller in another module, which does not declare it: XO0701, eager
	// or lazy (its body is checked before the caller's effects are)
	modtest.WriteFiles(t, ws, map[string]string{
		"lib/shapes/s.xo":  strings.Replace(lib, "fn (p: Pt) display() -> Str {\n", "fn (p: Pt) display() -> Str {\n  log.info(\"display\")\n", 1),
		"app/main/main.xo": "use example.com/lib/shapes\n\npub fn label(p: shapes.Pt) -> Str {\n  p.display()\n}\n\nfn main(os: Os) {\n  os.stdio.println(label(shapes.Pt{x: 1, y: 2}))\n}\n",
	})
	modtest.WriteFiles(t, ws, map[string]string{"lib/shapes/s.xo": "use std/log\n\n" + strings.Replace(strings.Replace(lib, "fn (p: Pt) display() -> Str {\n", "fn (p: Pt) display() -> Str {\n  log.info(\"display\")\n", 1), "pub fn unused() -> Int {\n  \"not an Int\"\n}\n", "", 1)})
	for _, lazy := range []string{"0", ""} {
		setenv(t, "XO_DEP_LAZY", lazy)
		r = xo(t, "build", "--json", "-o", filepath.Join(ws, "bin"), main).want(1)
		if !strings.Contains(r.out+r.err, "XO0701") {
			t.Errorf("XO_DEP_LAZY=%s: a pub caller of a logging display method: want XO0701:\n%s%s", lazy, r.out, r.err)
		}
	}
}

// TestDepLazySyntaxError (decision 0517): with XO_DEP_LAZY=1 a dependency's
// bodies are parsed as the build reaches them. A syntax error in a reached
// body is reported as itself, at its file and line, and fails the build;
// it was a fault ("a lazily parsed body ... does not parse") until
// 2026-11-05. A cold build and two on the warm caches report the same: the body
// that did not parse is never cached as an empty block (that build said
// XO0401 instead).
func TestDepLazySyntaxError(t *testing.T) {
	ws := t.TempDir()
	modtest.WriteFiles(t, ws, map[string]string{
		"xo.work":          "xo preview\nuse ./app\nuse ./lib\n",
		"app/xo.mod":       "module example.com/app\nedition preview\n",
		"app/main/main.xo": "use example.com/lib/num\n\nfn main(os: Os) {\n  os.stdio.println(\"${num.bad()}\")\n}\n",
		"lib/xo.mod":       "module example.com/lib\nedition preview\n",
		"lib/num/n.xo":     "pub fn good() -> Int {\n  1\n}\n\npub fn bad() -> Int {\n  let x = (1 +\n  x\n}\n",
	})
	setenv(t, "XO_WORK", filepath.Join(ws, "xo.work"))
	setenv(t, "XO_DEP_LAZY", "1")
	main := filepath.Join(ws, "app", "main")
	for i := 1; i <= 3; i++ {
		r := xo(t, "build", "-o", filepath.Join(ws, "bin"), main).want(1)
		out := r.out + r.err
		if strings.Contains(out, "fault") || !strings.Contains(out, "n.xo:7:") || !strings.Contains(out, "XO0161") || strings.Contains(out, "XO0401") || strings.Contains(out, "XO1201") {
			t.Errorf("build %d: want the syntax error at n.xo:7 (XO0161), no fault, XO0401 or XO1201:\n%s", i, out)
		}
	}
}
