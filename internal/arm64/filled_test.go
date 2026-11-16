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

// filledProg makes 128 MiB of lists with List.filled (decision 0671): 32
// List[UInt8] of 4 MiB (MODE bytes, the mem_set path) or 32 List[Int] of
// 512 Ki zeros (ints, the cleared words).
const filledProg = `fn main(os: Os) {
  let ints = (os.env.get("MODE") ?? "bytes") == "ints"
  var sum = 0
  for i in 0..32 {
    if ints {
      let l = List[Int].filled(524288, 0)
      sum += l.len() + l[i]
    } else {
      let l = List[UInt8].filled(4194304, UInt8(i))
      sum += l.len() + Int(l[i])
    }
  }
  os.stdio.println("${sum}")
}
`

// TestListFilledBulk guards List.filled's cost. On the Mac (2026-11-15)
// the bytes took 101M instructions with filled and 1.44G pushed; the
// Int zeros 98M and 258M pushed. A ceiling well under the push loop
// fails if filled falls back to a push per element. On darwin the
// ceiling is /usr/bin/time's instruction count, elsewhere user CPU time.
func TestListFilledBulk(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module filled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(filledProg), 0o644); err != nil {
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
	for _, c := range []struct {
		mode    string
		want    string
		maxInsn int64
		maxUser time.Duration
	}{
		{"bytes", "134218224", 300_000_000, 300 * time.Millisecond},
		{"ints", "16777216", 180_000_000, 300 * time.Millisecond},
	} {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("/usr/bin/time", "-l", bin)
		} else {
			cmd = exec.Command(bin)
		}
		cmd = testtmp.Guard(cmd)
		cmd.Env = append(os.Environ(), "MODE="+c.mode)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", c.mode, err, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != c.want {
			t.Fatalf("%s: stdout %q, want %s", c.mode, got, c.want)
		}
		if runtime.GOOS == "darwin" {
			m := instrRE.FindStringSubmatch(stderr.String())
			if m == nil {
				t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
			}
			n, _ := strconv.ParseInt(m[1], 10, 64)
			if n > c.maxInsn {
				t.Errorf("%s: %d instructions for 128 MiB of List.filled, over the %d ceiling: filled no longer fills in bulk", c.mode, n, c.maxInsn)
			}
			continue
		}
		if u := cmd.ProcessState.UserTime(); u > c.maxUser {
			t.Errorf("%s: %v of user CPU time for 128 MiB of List.filled, over %v: filled no longer fills in bulk", c.mode, u, c.maxUser)
		}
	}
}
