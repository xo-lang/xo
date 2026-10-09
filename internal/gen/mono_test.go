package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// generate checks and generates the program src under --mono.
func generateMono(t *testing.T, src string) *gen.Program {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	res, ok, err := build.Check([]string{dir}, false)
	if err != nil || !ok {
		t.Fatalf("check: %v %v", err, res.Diagnostics)
	}
	prog, err := gen.Generate(res, gen.Options{Mode: gen.Build})
	if err != nil {
		t.Fatal(err)
	}
	return prog
}

func goSource(p *gen.Program) string {
	var b strings.Builder
	for name, src := range p.Files {
		if strings.HasPrefix(name, "m/") {
			b.Write(src)
		}
	}
	return b.String()
}

// TestMonoDepthCap: polymorphic recursion requests ever larger types;
// specialization stops at the depth cap and the deepest copy calls the
// generic function. (Go itself rejects the program as an instantiation
// cycle, as it does without --mono; this checks the generator terminates.)
func TestMonoDepthCap(t *testing.T) {
	prog := generateMono(t, `
fn depth[T](x: T, n: Int) -> Int {
  if n == 0 {
    0
  } else {
    1 + depth([x], n - 1)
  }
}

fn main(os: Os) {
  os.stdio.println("${depth(1, 3)}")
}
`)
	if prog.Specs == 0 || prog.Specs > 8 {
		t.Fatalf("specs = %d, want 1..8", prog.Specs)
	}
	src := goSource(prog)
	if !strings.Contains(src, "F_depth[rt.List[") {
		t.Fatalf("no generic fallback call in:\n%s", src)
	}
}

// TestMonoClosureNotInlined: closures that can fail or contain `return`
// stay closures and go through the rt higher order functions.
func TestMonoClosureNotInlined(t *testing.T) {
	src := goSource(generateMono(t, `
fn f(xs: List[Int]) -> List[Int] ! Str {
  xs.map(fn(x) {
    if x < 0 {
      return Err("neg")
    }
    x
  })
}

fn g(xs: List[Int]) -> List[Int] {
  xs.map(fn(x) {
    if x < 0 {
      return 0
    }
    x
  })
}

fn h(xs: List[Int]) -> List[Int] { xs.map(fn(x) { x + 1 }) }

fn main(os: Os) {
  let r = f([1]).ok() ?? []
  os.stdio.println("${g([1]).len()} ${h([1]).len()} ${r.len()}")
}
`))
	if n := strings.Count(src, "rt.List_map["); n != 2 {
		t.Fatalf("rt.List_map calls = %d, want 2 (f and g)\n%s", n, src)
	}
	if !strings.Contains(src, "rt.ListFromSlice(") {
		t.Fatalf("h was not inlined\n%s", src)
	}
}

// TestMonoMapViewInPlace: an inlined higher order call or a for loop over
// `m.values()` or `m.keys()` walks the map in place; other uses still
// build the list (golden map_view_loops checks the behavior). A
// comparison of two to_lower calls lowers to rt.Str_eq_lower.
func TestMonoMapViewInPlace(t *testing.T) {
	src := goSource(generateMono(t, `
fn taken(m: Map[Int, Str], s: Str) -> Bool { m.values().any(fn(v) { v == s }) }

fn total(m: Map[Int, Str]) -> Int {
  var n = 0
  for k in m.keys() {
    n += k
  }
  n
}

fn kept(m: Map[Int, Str]) -> List[Str] { m.values() }

fn differ(a: Str, b: Str) -> Bool { a.to_lower() != b.to_lower() }

fn main(os: Os) {
  let m: Map[Int, Str] = {1: "a"}
  os.stdio.println("${taken(m, "a")} ${total(m)} ${kept(m).len()} ${differ("A", "a")}")
}
`))
	for _, w := range []string{"rt.Map_values_seq(", "rt.Map_keys_seq(", "!rt.Str_eq_lower("} {
		if !strings.Contains(src, w) {
			t.Errorf("generated code lacks %q\n%s", w, src)
		}
	}
	if n := strings.Count(src, "rt.Map_values["); n != 1 {
		t.Errorf("Map_values calls = %d, want 1 (kept)\n%s", n, src)
	}
}
