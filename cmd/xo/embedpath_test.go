package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbedPathFix checks XO1511's fix: `xo check --fix` rewrites
// SITE.read("index.html") to the key that ends with it,
// "static/index.html", and the program then checks with no warning.
func TestEmbedPathFix(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"app/xo.mod":            "module app\n",
		"app/static/index.html": "<html></html>\n",
		"app/main.xo":           "use std/embed\n\nconst SITE: embed.Dir = embed(\"static\")\n\nfn main(os: Os) {\n  let b = SITE.read(\"index.html\") ?? b\"\"\n  os.stdio.println(\"${b.len()}\")\n}\n",
	})
	app := filepath.Join(dir, "app")
	var out, errb bytes.Buffer
	if code := run(t, []string{"check", app}, &out, &errb); code != 0 || !strings.Contains(out.String()+errb.String(), "warning XO1511") {
		t.Fatalf("check: exit %d, want 0 with warning XO1511\n%s%s", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(t, []string{"check", "--fix", app}, &out, &errb); code != 0 {
		t.Fatalf("check --fix: exit %d\n%s%s", code, out.String(), errb.String())
	}
	b, _ := os.ReadFile(filepath.Join(app, "main.xo"))
	if !strings.Contains(string(b), `SITE.read("static/index.html")`) {
		t.Errorf("not fixed:\n%s", b)
	}
	out.Reset()
	errb.Reset()
	if code := run(t, []string{"check", app}, &out, &errb); code != 0 || strings.Contains(out.String()+errb.String(), "XO1511") {
		t.Errorf("after the fix: exit %d\n%s%s", code, out.String(), errb.String())
	}
}
