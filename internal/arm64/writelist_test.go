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

// writeListProg appends 131,072,000 bytes of a List[UInt8] to builders in
// runs of RUN bytes with BytesBuilder.write_list (decision 0667).
const writeListProg = `fn main(os: Os) {
  let run = Int.parse(os.env.get("RUN") ?? "65536") ?? 65536
  let total = 131072000
  var l: List[UInt8] = []
  for i in 0..run {
    l.push(UInt8(i % 251))
  }
  var sum = 0
  for _ in 0..total / run {
    var b = BytesBuilder.with_capacity(run)
    b.write_list(l, 0, run)
    let out = b.freeze()
    sum += Int(out[run - 1]) + out.len()
  }
  os.stdio.println("${sum}")
}
`

// TestWriteListBulk guards write_list's cost: 131 MB appended in 64 KiB
// runs (the copy path) and in 100 byte runs (eight bytes a word push).
// A write_byte per element took 1.99G and 2.12G instructions on the Mac
// (2026-11-15); write_list 77M and 555M. A ceiling well under the byte
// loop fails if write_list falls back to a push per byte. On darwin the
// ceiling is /usr/bin/time's instruction count, elsewhere user CPU time.
func TestWriteListBulk(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module writelist\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(writeListProg), 0o644); err != nil {
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
		run     int
		want    string
		maxInsn int64
		maxUser time.Duration
	}{
		{65536, "131120000", 400_000_000, 500 * time.Millisecond},
		{100, "260833280", 1_100_000_000, time.Second},
	} {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("/usr/bin/time", "-l", bin)
		} else {
			cmd = exec.Command(bin)
		}
		cmd = testtmp.Guard(cmd)
		cmd.Env = append(os.Environ(), "RUN="+strconv.Itoa(c.run))
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %d: %v\n%s", c.run, err, stderr.String())
		}
		if got := strings.TrimSpace(stdout.String()); got != c.want {
			t.Fatalf("run %d: stdout %q, want %s", c.run, got, c.want)
		}
		if runtime.GOOS == "darwin" {
			m := instrRE.FindStringSubmatch(stderr.String())
			if m == nil {
				t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
			}
			n, _ := strconv.ParseInt(m[1], 10, 64)
			if n > c.maxInsn {
				t.Errorf("runs of %d: %d instructions for 131 MB of write_list, over the %d ceiling: write_list no longer copies in bulk", c.run, n, c.maxInsn)
			}
			continue
		}
		if u := cmd.ProcessState.UserTime(); u > c.maxUser {
			t.Errorf("runs of %d: %v of user CPU time for 131 MB of write_list, over %v: write_list no longer copies in bulk", c.run, u, c.maxUser)
		}
	}
}
