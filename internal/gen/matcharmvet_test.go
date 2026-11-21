package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestMatchArmClosureCallVet: a match in return position whose arm is an
// immediately called closure lowers to `return func(...) {...}(tk, v).V`,
// a return that ends on a line beginning with "}"; the arm got a dead
// `break` after it, which go vet reports as unreachable code (task #570,
// progen arith seed 5690147).
func TestMatchArmClosureCallVet(t *testing.T) {
	dir := t.TempDir()
	src := `enum E {
  A(n: Int)
  C
}

fn f(p: Int) -> Int {
  if p > 0 {
    return (match E.C {
      A(n) => n + 1
      C => fn(x: Int) -> Int { x ^ (-1) }(p)
    })
  }
  0
}

fn main(os: Os) {
  os.stdio.println("${f(3)} ${f(-1)}")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(dir, "gotree")
	res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(dir, "prog"), Flags: []string{"--go-tree=" + tree}})
	if err != nil || res.Failed {
		t.Fatalf("the Go build failed: %v\n%s", err, buildText(res))
	}
	// the program's package is under m/ (as in TestArithProps)
	if out, err := run(tree, "go", "vet", ".", "./m/..."); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	out, err := run(dir, res.Binaries[xotool.Host()])
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "-4 0\n"; strings.TrimLeft(out, "\n") != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
