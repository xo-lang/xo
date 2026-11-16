package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestCoalesceDivergingVet: `opt ?? return x` (and a diverging block on
// the right of `??`) lowers without a statement after the block's
// terminator, which go vet reports as unreachable code (std/metrics'
// parse_proc_stat failed go vet ./std/rt on main, 2026-11-16).
func TestCoalesceDivergingVet(t *testing.T) {
	dir := t.TempDir()
	src := `fn first(s: Str) -> Int ! Str {
  let at = s.rfind(")") ?? return Err("no paren")
  Ok(at)
}

fn count(xs: List[Str]) -> Int {
  var n = 0
  for x in xs {
    let i = x.find("=") ?? { continue }
    let j = x.find(";") ?? {
      n += 100
      break
    }
    n += i + j
  }
  n
}

fn main(os: Os) {
  os.stdio.println("${first("a)b").ok() ?? -1} ${first("ab").ok() ?? -1}")
  os.stdio.println("${count(["a=b;", "x", "c=;", "d=e"])}")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(dir, "gotree")
	res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(dir, "prog"), Flags: []string{"--go-tree=" + tree}})
	if err != nil || res.Failed {
		t.Fatalf("the Go build failed: %v\n%s", err, diagText(res))
	}
	// the program's package is under m/ (as in TestArithProps)
	if out, err := run(tree, "go", "vet", ".", "./m/..."); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
	out, err := run(dir, res.Binaries[xotool.Host()])
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if want := "1 -1\n107\n"; strings.TrimLeft(out, "\n") != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
