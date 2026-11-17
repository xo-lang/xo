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

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// escFastProg checks 10,000,000 lines of 26 to 54 bytes for ESC: with
// Str.contains of one byte (MODE byte) or term.strip (MODE strip).
const escFastProg = `use std/term

fn main(os: Os) {
  let strip = (os.env.get("MODE") ?? "byte") == "strip"
  var lines: List[Str] = []
  for i in 0..64 {
    lines.push("line ${i}: " + "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ"[0..(18 + i % 28)])
  }
  var n = 0
  for k in 0..10000000 {
    let l = lines[k & 63]
    if strip {
      n += term.strip(l).len()
    } else if l.contains("\u{1b}") {
      n += 1
    }
  }
  os.stdio.println("${n}")
}
`

// TestEscCheckFast guards the one-byte paths of task #103 lever 5:
// scan.find_bytes calls find_byte for a one byte sub before
// find_bytes_n's prologue saves its registers, and term.strip checks
// for ESC before strip_esc's. On the Mac (2026-11-17, a whole program
// build) the byte mode took 1.07G instructions before and 0.66G after,
// the strip mode 0.97G and 0.83G; with std's store linked (the default
// once it is cached) 1.15G and 0.93G, 1.14G and 1.07G. darwin only
// (/usr/bin/time's instruction count).
func TestEscCheckFast(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("instruction counts from /usr/bin/time -l (darwin)")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module escfast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(escFastProg), 0o644); err != nil {
		t.Fatal(err)
	}
	// XO_SEPCOMP=0: the whole program, std inlined, whether or not std's
	// store is cached (a default build links the store when it is)
	res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_SEPCOMP=0"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	bin := res.Binaries[xotool.Host()]
	for _, c := range []struct {
		mode    string
		want    string
		maxInsn int64
	}{
		{"byte", "0", 850_000_000},
		{"strip", "390937500", 900_000_000},
	} {
		cmd := testtmp.Guard(exec.Command("/usr/bin/time", "-l", bin))
		cmd.Env = append(os.Environ(), "MODE="+c.mode)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", c.mode, err, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != c.want {
			t.Fatalf("%s: stdout %q, want %s", c.mode, got, c.want)
		}
		m := instrRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n > c.maxInsn {
			t.Errorf("%s: %d instructions for 10,000,000 ESC checks, over the %d ceiling: the one byte path saves registers again", c.mode, n, c.maxInsn)
		}
	}
}
