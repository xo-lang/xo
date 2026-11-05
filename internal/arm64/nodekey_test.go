package arm64

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
)

// nodeKeyFile is a source past 1 MiB whose `beta()` call starts at
// offset lo and whose `alpha()` call starts at lo + 2^20 - 1. The
// checker keyed a node by its file, start and end in 20 bits each, so
// file f's `alpha()` (and its `alpha`) had the key of the next file's
// `beta()` (and `beta`): one call's records were the other's, and a
// call could resolve to the other function (aws's 1.27 MB S3 client,
// XO1201 in unrelated functions, 2026-11-04;
// gotchas/node-key-aliased-past-1-mib.md). head holds the declarations;
// tag names the file's two functions.
func nodeKeyFile(t *testing.T, head, tag string, lo int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(head)
	// comment lines up to the offset where the text after them starts at
	pad := func(at int, before string) {
		to := at - len(before)
		n := to - b.Len()
		if n < 3 {
			t.Fatalf("%s: no room before offset %d", tag, at)
		}
		for n > 0 {
			k := n
			if k > 1000 {
				k = 1000
			}
			if n-k > 0 && n-k < 3 {
				k = n - 3
			}
			b.WriteString("//" + strings.Repeat("x", k-3) + "\n")
			n -= k
		}
		b.WriteString(before)
	}
	pad(lo, "fn lo_"+tag+"() -> Int { ")
	b.WriteString("beta() }\n")
	pad(lo+1<<20-1, "fn hi_"+tag+"() -> Int { ")
	b.WriteString("alpha() }\n")
	return b.String()
}

// TestNodeKeysPast1MiB builds two files past 1 MiB that call alpha and
// beta at the offsets whose keys aliased (nodeKeyFile), in either file
// order, and checks each call reaches its own function.
func TestNodeKeysPast1MiB(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	dir, err := testtmp.Dir("xo-nodekey-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	const lo = 20001
	headA := `fn alpha() -> Int {
  1
}

fn beta() -> Int {
  2
}

fn main(os: Os) {
  os.stdio.println("${hi_a()} ${lo_a()} ${hi_b()} ${lo_b()}")
}
`
	files := map[string]string{
		"xo.mod": "module nodekey\n",
		"a.xo":   nodeKeyFile(t, headA, "a", lo),
		"b.xo":   nodeKeyFile(t, "", "b", lo),
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := buildRun(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	const want = "1 2 1 2\n"
	if res.exit != 0 || res.stdout != want {
		t.Errorf("exit %d, stdout %q, want %q\nstderr:\n%s", res.exit, res.stdout, want, res.stderr)
	}
}
