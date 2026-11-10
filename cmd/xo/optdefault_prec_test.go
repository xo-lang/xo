package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOptionDefaultFixPrecedence: `xo check --fix` rewrites `.or(x)` to
// `??`, which binds loosest, so the rewrite is parenthesized wherever an
// operator next to it would take part of it (habits.xo infix_at; the
// owner's word count of 2026-11-10: `m.get(w).or(0) + 1` became
// `m.get(w) ?? 0 + 1`, `m.get(w) ?? (0 + 1)`, so every count was 1). The
// fixed program prints what the intended program prints.
func TestOptionDefaultFixPrecedence(t *testing.T) {
	t.Parallel()
	src := `fn main(os: Os) {
  let m: Map[Str, Int] = {"a": 5}
  let p = m.get("w").or(0) + 1
  let q = m.get("a").or(0) * 2
  let r = m.get("w").or(0) == 0
  let s = m.get("a").or(0).abs()
  let u = -m.get("a").or(0)
  let v = 1 + m.get("a").or(0)
  let w = m.get("a").or(0)
  os.stdio.println("${p} ${q} ${r} ${s} ${u} ${v} ${w} ${f(m)}")
}

fn f(m: Map[Str, Int]) -> Int {
  return m.get("z").or(3)
}
`
	dir := writeFiles(t, map[string]string{"app/main.xo": src})
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", "--fix", app}, &out, &errOut); code != 0 {
		t.Fatalf("check --fix: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	b, _ := os.ReadFile(filepath.Join(app, "main.xo"))
	for _, w := range []string{
		`let p = (m.get("w") ?? 0) + 1`,
		`let q = (m.get("a") ?? 0) * 2`,
		`let r = (m.get("w") ?? 0) == 0`,
		`let s = (m.get("a") ?? 0).abs()`,
		`let u = -(m.get("a") ?? 0)`,
		`let v = 1 + (m.get("a") ?? 0)`,
		`let w = m.get("a") ?? 0` + "\n",
		`return m.get("z") ?? 3` + "\n",
	} {
		if !strings.Contains(string(b), w) {
			t.Errorf("no %q in the fixed file:\n%s", w, b)
		}
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"run", app}, &out, &errOut); code != 0 || out.String() != "1 10 true 5 -5 6 5 3\n" {
		t.Errorf("fixed program: exit %d, output %q, want %q\n%s", code, out.String(), "1 10 true 5 -5 6 5 3\n", errOut.String())
	}
}
