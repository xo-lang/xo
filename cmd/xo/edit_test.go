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

// renameMembersProg exercises field, variant, and interface method
// renames: struct literal keys (one converted to an interface in a list),
// selectors, constructor arguments, patterns, a Json field that keeps
// its key, and a type with a same-named method that is not an
// implementation.
const renameMembersProg = `use std/json

struct Point {
  x: Int
  y: Int
}

enum Shape {
  Dot(at: Point)
  Line(a: Point, b: Point)
}

interface Area {
  fn area(self) -> Int
  fn label(self) -> Str
}

struct Rect {
  w: Int
  h: Int
}

fn (r: Rect) area() -> Int { r.w * r.h }
fn (r: Rect) label() -> Str { "rect" }

// area() without label(): not an Area, keeps its name
struct Wire {
  n: Int
}

fn (w: Wire) area() -> Int { w.n }

struct Rec {
  id: Int
  full_name: Str
} derive Json

enum Kind {
  Small
  Big
} derive Json

fn sum(xs: List[Area]) -> Int {
  var t = 0
  for x in xs {
    t += x.area()
  }
  t
}

fn report() -> Str {
  let p = Point{x: 1, y: 2}
  let s = Shape.Line(a: p, b: Point{x: 3, y: 4})
  let q = match s {
    Shape.Line(a, b) => a.x + b.y
    Shape.Dot(at: d) => d.x
  }
  let rs: List[Area] = [Rect{w: 2, h: 3}]
  "${sum(rs)} ${Wire{n: 4}.area()} ${json.encode_str(Rec{id: 1, full_name: "ada"})} ${p.x} ${q}"
}

fn main(os: Os) {
  os.stdio.println(report())
}
`

// TestEditRenameMembers renames fields, an enum variant, and an interface
// method with its implementations; the program then checks, prints the
// same, and keeps its test passing. A Json field keeps its key; a Json
// enum's variant is not renamed.
func TestEditRenameMembers(t *testing.T) {
	t.Parallel()
	dir := writeFiles(t, map[string]string{
		"xo.mod":       "module example.com/m\n",
		"main.xo":      renameMembersProg,
		"main_test.xo": "test \"report\" {\n  expect(report()) == \"6 4 {\\\"id\\\":1,\\\"full_name\\\":\\\"ada\\\"} 1 5\"\n}\n",
	})
	runOut := func() string {
		t.Helper()
		var out, errOut bytes.Buffer
		if code := run(t, []string{"run", dir}, &out, &errOut); code != 0 {
			t.Fatalf("run: exit %d: %s", code, errOut.String())
		}
		return out.String()
	}
	before := runOut()
	for _, c := range []struct{ old, new, gone, has string }{
		{"Point.x", "horiz", "p.x", "a.horiz + b.y"},
		{"Rect.w", "width", "Rect{w:", "Rect{width: 2"},
		{"Shape.Line", "Segment", "Shape.Line", "Shape.Segment(a: p"},
		{"Rec.full_name", "name", "full_name: Str", `name as "full_name": Str`},
		{"Area.area", "size", "x.area()", "fn (r: Rect) size()"},
	} {
		var out, errOut bytes.Buffer
		if code := run(t, []string{"edit", "--dir=" + dir, "rename", c.old, c.new}, &out, &errOut); code != 0 {
			t.Fatalf("rename %s %s: exit %d: %s%s", c.old, c.new, code, out.String(), errOut.String())
		}
		b, _ := os.ReadFile(filepath.Join(dir, "main.xo"))
		if strings.Contains(string(b), c.gone) || !strings.Contains(string(b), c.has) {
			t.Errorf("after rename %s %s (want no %q, a %q):\n%s", c.old, c.new, c.gone, c.has, b)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "main.xo"))
	if !strings.Contains(string(b), "fn (w: Wire) area()") {
		t.Errorf("Wire.area, not an Area implementation, was renamed:\n%s", b)
	}
	if after := runOut(); after != before {
		t.Errorf("output changed: %q, was %q", after, before)
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"test", dir}, &out, &errOut); code != 0 {
		t.Errorf("test after the renames: exit %d: %s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run(t, []string{"edit", "--dir=" + dir, "rename", "Kind.Small", "Tiny"}, &out, &errOut); code == 0 || !strings.Contains(out.String()+errOut.String(), "derives Json") {
		t.Errorf("renaming a Json enum's variant: exit %d: %s%s", code, out.String(), errOut.String())
	}
}
