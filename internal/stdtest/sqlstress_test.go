package stdtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestSqlStressNoLargeReads: the MySQL driver and the sql stress program's
// fake server ask each read for what the next packet lacks (4 KiB to
// 1 MiB). Asking 64 KiB every time made each read a large block (the
// read buffer is as large as asked), 93% of the program's bytes
// allocated and a resident set that grew 46% in 30 min where Go's twin
// stayed flat; 2000 statements then left mappings in the large-block
// cache (4 with the old driver alone), and now none. The growth left after
// the fix (69 to 88 MB in 30 min on linux/arm64 host 2, 68 MB of it on
// the free lists at exit) is the allocator keeping freed memory.
func TestSqlStressNoLargeReads(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the sql stress program")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sqlstress-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "stress")
	xb := testtmp.Guard(exec.Command(xo, "build", "--backend=native", "-o", bin, filepath.Join(root, "std", "evidence", "sql", "stress")))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	// 50 passes for each of 8 clients over one pool, no lifetime
	run := testtmp.Guard(exec.Command(bin, "0", "8", "0"))
	run.Env = append(os.Environ(), "XO_HEAPSTATS=1")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("stress: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "sql stress: 2000 statements") {
		t.Fatalf("no summary of 2000 statements:\n%s", text)
	}
	if !strings.Contains(text, "xo heapstats: large cache 0 mappings") {
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "large cache") {
				t.Errorf("reads made large blocks: %s (want 0 mappings)", line)
			}
		}
	}
}
