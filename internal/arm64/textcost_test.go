package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// atoiCostProg parses 3,000,000 signed decimal Strs of up to 8 digits.
const atoiCostProg = `fn main(os: Os) {
  var xs: List[Str] = []
  var seed = 7
  for _ in 0..1000 {
    seed = (seed * 1103515245 + 12345) % 2147483648
    xs.push("${seed % 100000000 - 50000000}")
  }
  var h = 0
  for i in 0..3000000 {
    h = (h + (Int.parse(xs[i % 1000]) ?? 0)) % 1000000007
  }
  os.stdio.println("${h}")
}
`

// trimCostProg trims 6,000,000 short ASCII Strs by ASCII cutsets of one to
// three bytes.
const trimCostProg = `fn main(os: Os) {
  let texts = ["  the quick brown fox  ", "***  a  ***", "abcabcabcabc", " \t mixed \t ", "   "]
  let cuts = [" ", " \t", "abc", "*", "xyz"]
  var h = 0
  for i in 0..3000000 {
    h += texts[i % 5].trim_chars(cuts[i % 5]).len() + texts[(i + 2) % 5].trim_chars(cuts[i % 5]).len()
  }
  os.stdio.println("${h}")
}
`

// matchCostProg matches 2,000,000 paths against patterns with byte
// classes and `?`.
const matchCostProg = `use std/path

fn main(os: Os) {
  let pats = ["[a-z]*.gz", "file.[tc]*", "a/?/c[0-9]", "[^x]?[a-c]*"]
  let names = ["ls.1.gz", "file.tar", "a/b/c7", "abc.txt", "zz"]
  var h = 0
  for i in 0..2000000 {
    if path.is_match(pats[i % 4], names[i % 5]).ok() ?? false {
      h += 1
    }
  }
  os.stdio.println("${h}")
}
`

// TestTextCost guards the cost of Int.parse and Str.trim_chars (task
// #302). On the Mac (2026-11-17) the atoi program ran 540M instructions
// with the overflow checked loop alone and 370M with strconv.Atoi's fast
// path for fewer than 19 digits (bench/perf/strconv atoi 1.46 of Go
// before); the trim program ran 801M with trim_chars an end trim then a
// start trim, two calls and two views, and 774M with both ends in one
// frame and one view (bench/perf/trimchars 1.62 of Go before). Each
// ceiling sits between the two. On darwin the ceiling is /usr/bin/time's
// instruction count, elsewhere user CPU time.
func TestTextCost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	cases := []struct {
		name, prog, want string
		maxInsn          int64
		maxUser          time.Duration
		what             string
	}{
		{"atoi", atoiCostProg, "-948823852", 450_000_000, 120 * time.Millisecond, "Int.parse lost strconv.Atoi's fast path"},
		{"match", matchCostProg, "400000", 1_600_000_000, 400 * time.Millisecond, "path.is_match decodes an ASCII byte of a class or `?` with a call (Go inlines utf8.DecodeRuneInString's ASCII test)"},
		{"trim", trimCostProg, "58800000", 788_000_000, 250 * time.Millisecond, "Str.trim_chars no longer trims both ends of an ASCII cutset in one pass"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module "+c.name+"cost\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(c.prog), 0o644); err != nil {
				t.Fatal(err)
			}
			res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64})
			if err != nil {
				t.Fatal(err)
			}
			if res.Failed {
				t.Fatalf("build failed:\n%s", res.Output)
			}
			bin := res.Binaries[xotool.Host()]
			var cmd *exec.Cmd
			if runtime.GOOS == "darwin" {
				cmd = exec.Command("/usr/bin/time", "-l", bin)
			} else {
				cmd = exec.Command(bin)
			}
			cmd = testtmp.Guard(cmd)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v\n%s", err, stderr.String())
			}
			if got := strings.TrimSpace(stdout.String()); got != c.want {
				t.Fatalf("stdout %q, want %q", got, c.want)
			}
			if runtime.GOOS == "darwin" {
				m := instrRE.FindStringSubmatch(stderr.String())
				if m == nil {
					t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
				}
				n, _ := strconv.ParseInt(m[1], 10, 64)
				t.Logf("%d instructions (ceiling %d)", n, c.maxInsn)
				if n > c.maxInsn {
					t.Errorf("%d instructions, over the %d ceiling: %s", n, c.maxInsn, c.what)
				}
				return
			}
			if u := cmd.ProcessState.UserTime(); u > c.maxUser {
				t.Errorf("%v of user CPU time, over %v: %s", u, c.maxUser, c.what)
			}
		})
	}
}
