package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOptionDefaultHabits checks the habit notes for an Option default
// written as another language's method (`.or(x)`, `.unwrap_or(x)`,
// `.get_or_else(x)`, `.value_or(x)`, `.or_else(f)`): each is XO0601
// with the note that Xo writes `opt ?? x`, `xo check --fix` rewrites
// them to `??` and the program then checks, and `.or_else` with a fn
// literal keeps the note with no fix.
func TestOptionDefaultHabits(t *testing.T) {
	t.Parallel()
	src := `fn dflt() -> Str { "d" }

fn main(os: Os) {
  let a = os.env.get("PORT").or("8090")
  let b = os.env.get("A").unwrap_or("x")
  let c = os.env.get("B").get_or_else("y")
  let e = os.env.get("C").value_or("z")
  let f = os.env.get("D").or_else(dflt)
  os.stdio.println("${a} ${b} ${c} ${e} ${f}")
}
`
	dir := writeFiles(t, map[string]string{"app/main.xo": src, "lit/main.xo": "fn main(os: Os) {\n  let g = os.env.get(\"E\").or_else(fn() { \"w\" })\n  os.stdio.println(g)\n}\n"})
	app := filepath.Join(dir, "app")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", app}, &out, &errOut); code != 1 {
		t.Fatalf("check: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	all := out.String() + errOut.String()
	for _, m := range []string{"or", "unwrap_or", "get_or_else", "value_or", "or_else"} {
		if !strings.Contains(all, "error XO0601: `Str?` has no field or method `"+m+"`") {
			t.Errorf("no XO0601 for .%s in\n%s", m, all)
		}
	}
	if n := strings.Count(all, "Xo writes `opt ?? "); n != 5 {
		t.Errorf("%d notes, want 5\n%s", n, all)
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--fix", app}, &out, &errOut); code != 0 {
		t.Fatalf("check --fix: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	b, _ := os.ReadFile(filepath.Join(app, "main.xo"))
	for _, w := range []string{`let a = os.env.get("PORT") ?? "8090"`, `let b = os.env.get("A") ?? "x"`, `let c = os.env.get("B") ?? "y"`, `let e = os.env.get("C") ?? "z"`, `let f = os.env.get("D") ?? dflt()`} {
		if !strings.Contains(string(b), w) {
			t.Errorf("no %q in the fixed file:\n%s", w, b)
		}
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"check", "--json", filepath.Join(dir, "lit")}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "Xo writes `opt ?? f()`") || strings.Contains(out.String(), `"edits"`) {
		t.Errorf("fn literal: exit %d\n%s%s", code, out.String(), errOut.String())
	}
}
