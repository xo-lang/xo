package defhash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/parser"
)

const common = `pub struct Point {
  pub x: Int
  pub y: Int
}

fn g(a: Int) -> Int { a + 1 }
`

// module writes a one package module and hashes it.
func module(t *testing.T, files map[string]string) []Def {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"xo.mod": "module example.com/m\nedition 2026\n"}
	for k, v := range files {
		all[k] = v
	}
	for name, src := range all {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, ".xo") {
			if _, errs := parser.ParseFile(name, []byte(src), 0); len(errs) > 0 {
				t.Fatalf("%s does not parse: %v\n%s", name, errs, src)
			}
		}
	}
	defs, err := Module(dir, "example.com/m")
	if err != nil {
		t.Fatal(err)
	}
	return defs
}

func hashOf(t *testing.T, defs []Def, name string) Def {
	t.Helper()
	ds := Find(defs, name)
	if len(ds) != 1 {
		t.Fatalf("%d definitions named %s in %+v", len(ds), name, defs)
	}
	return ds[0]
}

// TestStable: what does not change a definition's meaning keeps its
// hash.
func TestStable(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"renamed let", "fn f(a: Int) -> Int {\n  let x = g(a)\n  x * 2\n}\n", "fn f(a: Int) -> Int {\n  let total = g(a)\n  total * 2\n}\n"},
		{"renamed var and loop variable", "fn f(xs: List[Int]) -> Int {\n  var s = 0\n  for x in xs {\n    s += x\n  }\n  s\n}\n", "fn f(xs: List[Int]) -> Int {\n  var acc = 0\n  for item in xs {\n    acc += item\n  }\n  acc\n}\n"},
		{"renamed match binding", "fn f(o: Int?) -> Int {\n  match o {\n    Some(v) => v\n    None => 0\n  }\n}\n", "fn f(o: Int?) -> Int {\n  match o {\n    Some(inner) => inner\n    None => 0\n  }\n}\n"},
		{"renamed closure parameter", "fn f(xs: List[Int]) -> List[Int] { xs.map(fn(n) { n + 1 }) }\n", "fn f(xs: List[Int]) -> List[Int] { xs.map(fn(item) { item + 1 }) }\n"},
		{"struct pattern shorthand", "fn f(p: Point) -> Int {\n  let Point{x, y} = p\n  x + y\n}\n", "fn f(p: Point) -> Int {\n  let Point{x: a, y: b} = p\n  a + b\n}\n"},
		{"renamed type parameter", "fn f[T](xs: List[T]) -> Int { xs.len() }\n", "fn f[Elem](xs: List[Elem]) -> Int { xs.len() }\n"},
		{"comments, docs, layout", "/// Doubles.\nfn f(a: Int) -> Int { a * 2 }\n", "// a comment\nfn f(a: Int) -> Int {\n  // twice\n  a * 2\n}\n"},
		{"renamed import", "use std/strings\n\nfn f(s: Str) -> Str { strings.trim(s) }\n", "use std/strings as st\n\nfn f(s: Str) -> Str { st.trim(s) }\n"},
	}
	join := func(s string) string {
		if strings.HasPrefix(s, "use ") {
			i := strings.Index(s, "\n\n") + 2
			return s[:i] + common + s[i:]
		}
		return common + s
	}
	for _, c := range cases {
		a := hashOf(t, module(t, map[string]string{"m.xo": join(c.a)}), "f")
		b := hashOf(t, module(t, map[string]string{"m.xo": join(c.b)}), "f")
		if a.Hash != b.Hash || a.Own != b.Own {
			t.Errorf("%s: hash changed\n%s\n%s", c.name, a.Hash, b.Hash)
		}
	}
	// a definition's own name is not part of its content
	a := hashOf(t, module(t, map[string]string{"m.xo": common + "fn f(a: Int) -> Int { a * 2 }\n"}), "f")
	b := hashOf(t, module(t, map[string]string{"m.xo": common + "fn double(a: Int) -> Int { a * 2 }\n"}), "double")
	if a.Hash != b.Hash {
		t.Errorf("renaming the function changed its hash")
	}
	// the module path is not either (a /v2 path compares with v1)
	d1 := module(t, map[string]string{"m.xo": common + "fn f(a: Int) -> Int { g(a) }\n"})
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/m/v2\nedition 2026\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "m.xo"), []byte(common+"fn f(a: Int) -> Int { g(a) }\n"), 0o644)
	d2, _ := Module(dir, "example.com/m/v2")
	if hashOf(t, d1, "f").Hash != hashOf(t, d2, "f").Hash {
		t.Errorf("the module path changed the hash")
	}
}

// TestSensitive: a semantic change alters the hash.
func TestSensitive(t *testing.T) {
	base := "fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: a, y: b}\n  g(p.x) - p.y\n}\n"
	cases := []struct{ name, b string }{
		{"literal", "fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: a, y: b}\n  g(p.x) - p.y + 1\n}\n"},
		{"operator", "fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: a, y: b}\n  g(p.x) + p.y\n}\n"},
		{"swapped locals", "fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: b, y: a}\n  g(p.x) - p.y\n}\n"},
		{"other field", "fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: a, y: b}\n  g(p.y) - p.y\n}\n"},
		{"renamed parameter (named arguments see it)", "fn f(a: Int, c: Int) -> Int {\n  let p = Point{x: a, y: c}\n  g(p.x) - p.y\n}\n"},
		{"effect", "fn f(a: Int, b: Int) -> Int uses clock {\n  let p = Point{x: a, y: b}\n  g(p.x) - p.y\n}\n"},
		{"pub", "pub fn f(a: Int, b: Int) -> Int {\n  let p = Point{x: a, y: b}\n  g(p.x) - p.y\n}\n"},
	}
	want := hashOf(t, module(t, map[string]string{"m.xo": common + base}), "f").Hash
	for _, c := range cases {
		if got := hashOf(t, module(t, map[string]string{"m.xo": common + c.b}), "f").Hash; got == want {
			t.Errorf("%s: hash unchanged", c.name)
		}
	}
	// a field renamed in the struct changes the struct and, through it,
	// every definition using it, though their own text is the same
	old := module(t, map[string]string{"m.xo": common + base})
	renamed := module(t, map[string]string{"m.xo": strings.Replace(common, "fn g(a: Int) -> Int { a + 1 }", "fn g(a: Int) -> Int { a + 2 }", 1) + base})
	fo, fn := hashOf(t, old, "f"), hashOf(t, renamed, "f")
	if fo.Own != fn.Own || fo.Hash == fn.Hash {
		t.Errorf("a changed callee: own %v, hash %v", fo.Own == fn.Own, fo.Hash == fn.Hash)
	}
	changes := Diff(old, renamed)
	var got []string
	for _, c := range changes {
		got = append(got, c.Describe())
	}
	if strings.Join(got, "; ") != "private fn example.com/m.f changed through example.com/m.g; changed private fn example.com/m.g" {
		t.Errorf("diff: %q", got)
	}
}

// TestRecursion: mutually recursive definitions hash deterministically,
// and a change to one changes both.
func TestRecursion(t *testing.T) {
	src := "fn even(n: Int) -> Bool {\n  if n == 0 { true } else { odd(n - 1) }\n}\n\nfn odd(n: Int) -> Bool {\n  if n == 0 { false } else { even(n - 1) }\n}\n\nfn fact(n: Int) -> Int {\n  if n == 0 { 1 } else { n * fact(n - 1) }\n}\n"
	a := module(t, map[string]string{"m.xo": src})
	for i := 0; i < 5; i++ {
		b := module(t, map[string]string{"m.xo": src})
		for j := range a {
			if a[j].Hash != b[j].Hash {
				t.Fatalf("%s: hash differs between runs", a[j].ID)
			}
		}
	}
	changed := module(t, map[string]string{"m.xo": strings.Replace(src, "{ false }", "{ true }", 1)})
	if hashOf(t, a, "even").Hash == hashOf(t, changed, "even").Hash || hashOf(t, a, "even").Own != hashOf(t, changed, "even").Own {
		t.Errorf("a change to odd does not reach even through the cycle")
	}
	if hashOf(t, a, "fact").Hash != hashOf(t, changed, "fact").Hash {
		t.Errorf("fact changed")
	}
	// methods, cross package references, Find and Nearest
	defs := module(t, map[string]string{
		"m.xo":       "use example.com/m/geo\n\nfn f(p: geo.Point) -> Int { p.norm() + geo.origin().x }\n",
		"geo/geo.xo": "pub struct Point {\n  pub x: Int\n}\n\nfn (p: Point) norm() -> Int { p.x }\n\npub fn origin() -> Point { Point{x: 0} }\n",
	})
	f := hashOf(t, defs, "f")
	if strings.Join(f.Refs, ",") != "example.com/m/geo.Point,example.com/m/geo.origin" {
		t.Errorf("refs of f: %v", f.Refs)
	}
	if len(Find(defs, "geo.origin")) != 1 || len(Find(defs, "Point.norm")) != 1 || len(Find(defs, "example.com/m/geo.Point")) != 1 {
		t.Errorf("Find: %+v", defs)
	}
	if n := Nearest(defs, "orign", 3); len(n) == 0 || n[0] != "origin" {
		t.Errorf("Nearest: %v", n)
	}
	// the receiver's name is local
	other := module(t, map[string]string{
		"m.xo":       "use example.com/m/geo\n\nfn f(p: geo.Point) -> Int { p.norm() + geo.origin().x }\n",
		"geo/geo.xo": "pub struct Point {\n  pub x: Int\n}\n\nfn (pt: Point) norm() -> Int { pt.x }\n\npub fn origin() -> Point { Point{x: 0} }\n",
	})
	if hashOf(t, defs, "Point.norm").Hash != hashOf(t, other, "Point.norm").Hash {
		t.Errorf("renaming the receiver changed the method's hash")
	}
}
