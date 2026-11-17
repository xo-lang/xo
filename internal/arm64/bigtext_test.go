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

// bigTextProg writes a 2048 bit BigInt in decimal 20,000 times
// (bench/perf/big text's number).
const bigTextProg = `use std/big

fn next(x: Int) -> Int { (x * 1103515245 + 12345) % 2147483648 }

fn main(os: Os) {
  var s = 1
  var b = StrBuilder.new()
  for i in 0..512 {
    s = next(s)
    let d = (s >> 8) % 16
    let k = if i == 0 && d == 0 { 1 } else { d }
    b.write("0123456789abcdef"[k..(k + 1)])
  }
  let a = big.parse(b.freeze(), 16).ok() ?? fault("hex")
  var sum = 0
  for _ in 0..20000 {
    sum += a.text().len()
  }
  os.stdio.println("${sum}")
}
`

// TestBigTextFast guards task #4: convert_words' base-10 digit loop
// (BigInt.text) runs on wrapping operations and hoisted bounds, not
// checked ones. On the Mac (2026-11-17, a whole program build) 20,000
// texts took 931,953,259 instructions before and 836,024,541 after; linux/arm64
// host 1, bench/perf/big text: 0.935 of before (xo bench --ab, 3
// rounds). darwin only (/usr/bin/time's instruction count).
func TestBigTextFast(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("instruction counts from /usr/bin/time -l (darwin)")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module bigtext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(bigTextProg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_SEPCOMP=0"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	cmd := testtmp.Guard(exec.Command("/usr/bin/time", "-l", res.Binaries[xotool.Host()]))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != bigTextWant {
		t.Fatalf("stdout %q, want %s", got, bigTextWant)
	}
	m := instrRE.FindStringSubmatch(stderr.String())
	if m == nil {
		t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	t.Logf("%d instructions", n)
	if n > bigTextMaxInsn {
		t.Errorf("%d instructions for 20,000 texts of a 2048 bit BigInt, over the %d ceiling: the digit loop checks overflow again", n, bigTextMaxInsn)
	}
}

const bigTextWant = "12340000"

const bigTextMaxInsn = 880_000_000
