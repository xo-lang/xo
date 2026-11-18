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

// sortStrProg sorts 200,000 Strs that share the prefix "key" (as
// bench/perf/sort strs does) with List.sort.
const sortStrProg = `fn main(os: Os) {
  var xs: List[Str] = List.with_capacity(200000)
  var seed = 7
  for _ in 0..200000 {
    seed = (seed * 1103515245 + 12345) % 2147483648
    xs.push("key${seed % 1000000}")
  }
  let s = xs.sort()
  var h = 0
  for x in s {
    h = (h * 31 + x.len()) % 1000000007
  }
  os.stdio.println("${s[0]} ${s[s.len() - 1]} ${h}")
}
`

// TestListSortStrCost guards the cost of sorting Strs (task #302). On the
// Mac (2026-11-17) the program ran 293M instructions with the merge of
// indices by 8 byte prefixes and 209M with the Int keyed pdqsort that
// skips the common prefix (bench/perf/sort strs 1.42 of Go before, 0.96
// after). The ceiling sits between them. On darwin the ceiling is
// /usr/bin/time's instruction count, elsewhere user CPU time.
func TestListSortStrCost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module sortstr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(sortStrProg), 0o644); err != nil {
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
	if got, want := strings.TrimSpace(stdout.String()), "key0 key999993 78600059"; got != want {
		t.Fatalf("stdout %q, want %q", got, want)
	}
	const maxInsn = 245_000_000
	const maxUser = 150 * time.Millisecond
	if runtime.GOOS == "darwin" {
		m := instrRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if n > maxInsn {
			t.Errorf("%d instructions to sort 200,000 Strs, over the %d ceiling: List.sort of Strs no longer sorts Int keys", n, maxInsn)
		}
		return
	}
	if u := cmd.ProcessState.UserTime(); u > maxUser {
		t.Errorf("%v of user CPU time to sort 200,000 Strs, over %v: List.sort of Strs no longer sorts Int keys", u, maxUser)
	}
}
