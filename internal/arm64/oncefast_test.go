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

// onceFastProg reads a done task.Once 10,000,000 times.
const onceFastProg = `use std/task

fn main(os: Os) {
  let o = task.once[Int, Never]()
  var sum = 0
  for i in 0..10000000 {
    sum += o.get(fn() { 42 }) + i % 2
  }
  os.stdio.println("${sum}")
}
`

// TestOnceGetFast guards task #103 lever 7: a done Once is read under
// the Mutex's read lock (with_read), which copies the value out and
// writes nothing back, not under the write lock. On the Mac (2026-11-17,
// a whole program build) 10,000,000 gets took 6.71G instructions
// before and 2.97G after. Task #419 (2026-11-19): a done Once publishes
// its value's box (Once.done) and get reads it with one load and no lock,
// 0.44G; the ceiling, 0.9G, fails if get takes any lock again. darwin
// only (/usr/bin/time's instruction count).
func TestOnceGetFast(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("instruction counts from /usr/bin/time -l (darwin)")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module oncefast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(onceFastProg), 0o644); err != nil {
		t.Fatal(err)
	}
	// XO_SEPCOMP=0: the whole program, std inlined, whether or not std's
	// store is cached
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
	if got := strings.TrimSpace(stdout.String()); got != "425000000" {
		t.Fatalf("stdout %q, want 425000000", got)
	}
	m := instrRE.FindStringSubmatch(stderr.String())
	if m == nil {
		t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	t.Logf("%d instructions", n)
	if n > onceMaxInsn {
		t.Errorf("%d instructions for 10,000,000 gets of a done Once, over the %d ceiling: get takes a lock again (Once.done unpublished?)", n, onceMaxInsn)
	}
}

const onceMaxInsn = 900_000_000
