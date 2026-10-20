package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const editProg = `fn load(id: Int) -> Int {
  id + 1
}

fn twice() -> Int {
  load(1) + load(2)
}
`

func TestEditCLI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		code int
		want string // in the edited main.xo, or in stdout when rejected
	}{
		{"rename", []string{"rename", "load", "fetch"}, 0, "fn fetch(id: Int)"},
		{"change-sig", []string{"change-sig", "load", "(id: Int, step: Int = 1) -> Int"}, 0, "fn load(id: Int, step: Int = 1) -> Int"},
		{"rejected", []string{"change-sig", "load", "(id: Int, step: Int) -> Int"}, 1, `"ok": false`},
		{"dry run", []string{"--dry-run", "rename", "load", "fetch"}, 0, `"written": false`},
		// a dry run shows its changes as a diff (decision 0289)
		{"dry run diff", []string{"--dry-run", "rename", "load", "fetch"}, 0, `\n+fn fetch(id: Int) -> Int {\n`},
		{"unknown op", []string{"bogus"}, 2, ""},
		{"unknown name", []string{"rename", "nosuch", "x"}, 2, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"main.xo": editProg})
			var out, errOut bytes.Buffer
			code := run(t, append([]string{"edit", "--dir=" + dir}, c.args...), &out, &errOut)
			if code != c.code {
				t.Fatalf("exit %d, want %d: %s %s", code, c.code, out.String(), errOut.String())
			}
			if c.want == "" {
				return
			}
			got := out.String()
			// a dry run's answer is its summary; otherwise the edited file
			if code == 0 && !strings.Contains(c.want, `"`) && c.args[0] != "--dry-run" {
				b, _ := os.ReadFile(filepath.Join(dir, "main.xo"))
				got = string(b)
				var res map[string]any
				if err := json.Unmarshal(out.Bytes(), &res); err != nil || res["ok"] != true {
					t.Errorf("summary = %s (%v)", out.String(), err)
				}
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("want %q in:\n%s", c.want, got)
			}
		})
	}
}

const renameTypeProg = `pub struct Point {
  pub x: Int
  pub y: Int
}

fn (p: Point) sum() -> Int {
  p.x + p.y
}

fn origin() -> Point {
  Point{x: 0, y: 0}
}

enum Shape {
  Dot(at: Point)
  Line(a: Point, b: Point)
}

fn first(s: Shape) -> Point {
  match s {
    Shape.Dot(at) => at
    Shape.Line(a, _) => a
  }
}

fn all(ps: List[Point]) -> Point? {
  ps.first()
}

fn run() -> Int {
  let s = Shape.Dot(at: origin())
  first(s).sum() + (all([Point{x: 1, y: 2}]) ?? origin()).sum()
}
`

// TestEditRenameType covers xo edit rename of a type (decision 0289):
// every place a type name is written (declaration, parameter and result
// types, struct literals, a receiver, variants in construction and
// patterns, a type argument), in two files, rewritten through the editor's
// identifier resolution; the program checks clean after.
func TestEditRenameType(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"main.xo":  renameTypeProg,
		"other.xo": "fn far(p: Point) -> Shape {\n  Shape.Line(a: p, b: origin())\n}\n",
	})
	for _, c := range []struct{ old, new string }{{"Point", "Pos"}, {"Shape", "Figure"}} {
		var out, errOut bytes.Buffer
		if code := run(t, []string{"edit", "--dir=" + dir, "rename", c.old, c.new}, &out, &errOut); code != 0 {
			t.Fatalf("rename %s %s: exit %d: %s%s", c.old, c.new, code, out.String(), errOut.String())
		}
		for _, f := range []string{"main.xo", "other.xo"} {
			b, _ := os.ReadFile(filepath.Join(dir, f))
			if strings.Contains(string(b), c.old) || !strings.Contains(string(b), c.new) {
				t.Errorf("after rename %s %s, %s:\n%s", c.old, c.new, f, b)
			}
		}
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"check", dir}, &out, &errOut); code != 0 {
		t.Errorf("check after the renames: exit %d: %s%s", code, out.String(), errOut.String())
	}
}
