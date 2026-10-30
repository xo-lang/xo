package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

const mutexWithProg = `fn fill(m: Mutex[List[Int]], n: Int) {
  for i in 0..n {
    m.with(fn(var l: List[Int]) { l.push(i) })
  }
}

fn main(os: Os) {
  let n = Int.parse(os.env.get("N") ?? "1000") ?? 0
  let m = Mutex[List[Int]].new([])
  fill(m, n)
  os.stdio.println("${m.with(fn(l: List[Int]) -> Int { l.len() })}")
}
`

var instrRE = regexp.MustCompile(`(\d+)\s+instructions retired`)

// TestMutexWithPushLinear pushes 200,000 values, one per Mutex.with:
// the list moves into the closure, so each push is amortized constant.
// It was quadratic (2026-10-30, Xo Lang 2's report: 31.4G instructions
// against 120M now): borrow.xo made the payload list.pop took out of the
// mutex's list a borrowed projection, because the tuple's other field
// (the list pushed back after the call) was used after the call, and a
// path around the loop reached the payload's own read, so the caller
// kept a reference and the closure's first push copied the whole list.
// On darwin the ceiling is /usr/bin/time's instruction count, elsewhere
// the user CPU time.
func TestMutexWithPushLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module mxpush\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(mutexWithProg), 0o644); err != nil {
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
	cmd.Env = append(os.Environ(), "N=200000")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "200000" {
		t.Fatalf("stdout %q, want 200000", got)
	}
	if runtime.GOOS == "darwin" {
		m := instrRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		// 120M with the move; the copy per push was 31.4G
		if n > 2_000_000_000 {
			t.Errorf("%d instructions for 200,000 pushes in Mutex.with, over the 2G ceiling: each push copies the list again", n)
		}
		return
	}
	if u := cmd.ProcessState.UserTime(); u > 2*time.Second {
		t.Errorf("%v of user CPU time for 200,000 pushes in Mutex.with, over 2 s: each push copies the list again", u)
	}
}
