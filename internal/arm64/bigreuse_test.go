package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// bigReuseProg makes a 48 MiB Str and a 48 MiB Bytes, then frees both, 10
// times (bench/perf/repeat's shape at a third of its size).
const bigReuseProg = `fn main(os: Os) {
  var sum = 0
  for _ in 0..10 {
    let s = "x".repeat(48 << 20)
    let b = "y".bytes().repeat(48 << 20)
    sum += s.len() + b.len() + Int(b[b.len() - 1])
  }
  os.stdio.println("${sum}")
}
`

// TestBigBlockReuse guards decision 0713: a freed large block a recent
// request could use stays in the large block cache past its 64 MiB bound,
// so a program that makes and frees blocks of one large size again and
// again reuses their mappings, as Go's heap does, instead of mapping and
// faulting in every page afresh. bench/perf/repeat/byte1 (160 MiB blocks)
// ran 3.2 of Go on linux/amd64 and 0.29 s against Go's 0.12 s on the Mac
// with 205,239 page reclaims (Go 21,210); with the rule, 0.08 s and 41,383.
// Here a round's blocks are 96 MiB of pages: 10 x 96 MiB without reuse,
// one round's with it; the ceiling is 1.5 x 96 MiB of minor faults. The
// first round's blocks went back to the system before the want ring held
// their size, so the second round faulted in again (Mac 13,159 faults of
// 16 KiB, 2.1 rounds; 7,015 once take made the table on a program's first
// large request, 2026-11-19).
func TestBigBlockReuse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("minor fault counts from getrusage on darwin and linux")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module bigreuse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(bigReuseProg), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	if got, want := strings.TrimSpace(stdout.String()), "1006634170"; got != want {
		t.Fatalf("stdout %q, want %q", got, want)
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Skip("no rusage")
	}
	limit := int64(3 * 48 << 20 / os.Getpagesize())
	t.Logf("%d minor faults (ceiling %d)", ru.Minflt, limit)
	if int64(ru.Minflt) > limit {
		t.Errorf("%d minor faults, over %d: a freed large block of a size the program asks for again went back to the system (heapos.xo keep, decision 0713)", ru.Minflt, limit)
	}
}
