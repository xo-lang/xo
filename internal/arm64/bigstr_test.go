package arm64

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
)

// bigStrSrc is a program with one string past 2 GiB and views 2 GiB or
// more into it. A view's offset is the high 32 bits of its length word
// (decision 0330), so there the word is negative: the runtime read it
// with an arithmetic shift (its bytes 4 GiB early, a fault) and tested
// "is a view" with a signed compare (a view of a string held only by
// the view appended in place, over the start of the string). xo's own
// assembly text past 2 GiB (a huge windows/amd64 frame's location
// lists) came out as NUL bytes the same way
// (gotchas/str-view-offset-sign.md).
const bigStrSrc = `fn big() -> Str {
  // 2 GiB + 1 MiB of "abcdefgh"
  "abcdefgh".repeat(268566528)
}

fn main(os: Os) {
  let at = 2147483648 + 8
  let s = big()
  os.stdio.println("len ${s.len()}")
  let v = s[at..at + 8]
  os.stdio.println("view ${v} ${v == "abcdefgh"}")
  os.stdio.println("concat ${s[at + 3..at + 5] + s[at + 6..at + 7]}")
  os.stdio.println("interp ${v}${7}")
  os.stdio.println("after ${s[at + 8..at + 16]}")
  // the view is the string's only holder: an append must not write in
  // place, at the string's start
  let w = big()[at + 3..at + 8]
  let x = w + "!"
  os.stdio.println("alone ${x}")
}
`

const bigStrWant = `len 2148532224
view abcdefgh true
concat deg
interp abcdefgh7
after abcdefgh
alone defgh!
`

// TestStrViewPast2GiB runs bigStrSrc (about 2.2 GB of memory at its
// peak).
func TestStrViewPast2GiB(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds a program that allocates 2.2 GB")
	}
	dir, err := testtmp.Dir("xo-bigstr-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module bigstr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(bigStrSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := buildRun(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.exit != 0 || res.stdout != bigStrWant {
		t.Errorf("exit %d, stdout:\n%s\nwant:\n%s\nstderr:\n%s", res.exit, res.stdout, bigStrWant, res.stderr)
	}
}

// bigJoinSrc joins two strings of 2 GiB + 1 MiB: past the 4 GiB a Str
// holds, so it faults; raw.str_join set the length's low 32 bits, and the
// result came out silently cut (xo's 4.3 GB location lists of a 24,000
// line function lost their end label; gotchas/str-over-4gib-truncated.md).
const bigJoinSrc = `fn main(os: Os) {
  let s = "abcdefgh".repeat(268566528)
  let j = [s, s].join("")
  os.stdio.println("joined ${j.len()}")
}
`

// TestStrJoinPast4GiB runs bigJoinSrc (about 2.2 GB of memory: the
// check comes before the join's allocation).
func TestStrJoinPast4GiB(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds a program that allocates 2.2 GB")
	}
	dir, err := testtmp.Dir("xo-bigjoin-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module bigjoin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(bigJoinSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := buildRun(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.exit == 0 || strings.Contains(res.stdout, "joined") || !strings.Contains(res.stderr, "strings: Join output over 4 GiB") {
		t.Errorf("exit %d, stdout %q, want the join's 4 GiB fault; stderr:\n%s", res.exit, res.stdout, res.stderr)
	}
}
