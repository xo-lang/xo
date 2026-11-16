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

// TestSqliteBusyOnFewWorkers: 64 clients over 8 pools of one database
// file on 2 workers, 50 passes each, 10 times (the old driver failed 2
// runs in 3, each within a second). SQLite's busy handler slept inside
// the C call, holding its worker, and Xo's preemption signal cut its
// nanosleep short: with every worker asleep there the task holding the
// lock could not run, and SQLITE_BUSY came back. The
// driver now retries a busy step in Xo, parked on the runtime's timers.
func TestSqliteBusyOnFewWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the sqlite stress program")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sqlitestress-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	bin := filepath.Join(work, "stress")
	xb := testtmp.Guard(exec.Command(xo, "build", "--backend=native", "-o", bin, filepath.Join(root, "std", "evidence", "sqlite", "stress")))
	xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if out, err := xb.CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	for round := range 10 {
		run := testtmp.Guard(exec.Command(bin, "0", "64"))
		run.Env = append(os.Environ(), "XO_THREADS=2", "TMPDIR="+work)
		out, err := run.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "sqlite stress: 28800 statements") {
			t.Fatalf("round %d: %v\n%s", round, err, out)
		}
	}
}
