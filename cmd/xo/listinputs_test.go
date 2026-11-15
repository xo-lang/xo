package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// TestBuildListInputs checks `xo build --list-inputs` (owner, 2026-11-15,
// from the agents' survey): the program's file, the std modules it imports
// with their std/native twins, and the prelude's own, sorted, in text and
// as JSON, with nothing built.
func TestBuildListInputs(t *testing.T) {
	t.Parallel()
	app := filepath.Join(t.TempDir(), "app")
	modtest.WriteFiles(t, app, map[string]string{
		"xo.mod": "module example.com/app\nedition preview\n",
		"main.xo": `use std/csv

fn main(os: Os) {
  let rows = csv.parse("a,b\n").ok() ?? []
  os.stdio.println("${rows.len()}")
}
`,
	})
	out := filepath.Join(t.TempDir(), "prog")
	r := xo(t, "build", "--list-inputs", "-o", out, app).want(0)
	lines := strings.Split(strings.TrimSpace(r.out), "\n")
	has := map[string]bool{}
	for i, l := range lines {
		has[l] = true
		if i > 0 && lines[i-1] >= l {
			t.Errorf("not sorted at %q after %q", l, lines[i-1])
		}
	}
	main := ""
	for _, l := range lines {
		if strings.HasSuffix(l, "/main.xo") {
			main = l
		}
	}
	if main == "" {
		t.Errorf("no main.xo in:\n%s", r.out)
	}
	for _, f := range []string{"std/csv.xo", "std/native/csv.xo", "std/native/prelude.xo", "std/task.xo"} {
		if !has[f] {
			t.Errorf("%s missing from:\n%s", f, r.out)
		}
	}
	if _, err := os.Stat(out); err == nil {
		t.Errorf("--list-inputs built %s", out)
	}
	r = xo(t, "build", "--list-inputs", "--json", app).want(0)
	var rep struct {
		Files []struct {
			Path   string `json:"path"`
			Module string `json:"module"`
			Std    bool   `json:"std"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(r.out), &rep); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, r.out)
	}
	if len(rep.Files) != len(lines) {
		t.Errorf("JSON has %d files, text %d", len(rep.Files), len(lines))
	}
	for _, f := range rep.Files {
		if f.Path == "std/csv.xo" && (!f.Std || f.Module != "std/csv") {
			t.Errorf("std/csv.xo: %+v", f)
		}
		if f.Path == main && (f.Std || f.Module != "example.com/app") {
			t.Errorf("main.xo: %+v", f)
		}
	}
}
