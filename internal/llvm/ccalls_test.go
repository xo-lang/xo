package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestRuntimeCCalls runs testdata/rt/ccalls on both native backends
// (decision 0675, task #94): a C nanosleep beside spinning tasks is not cut
// short by the preemption signal, and tasks blocked in C do not keep a task
// spawned after them from running.
func TestRuntimeCCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX nanosleep")
	}
	dir := filepath.Join("testdata", "rt", "ccalls")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be, Env: libcEnv()})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if res.Dir != "" {
				defer os.RemoveAll(res.Dir)
			}
			if res.Failed {
				t.Fatalf("build failed:\n%s", res.Output)
			}
			bin := res.Binaries[xotool.Host()]
			run := func(what, threads string) string {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				cmd := testtmp.Guard(exec.CommandContext(ctx, bin, what))
				cmd.Env = append(os.Environ(), "XO_THREADS="+threads)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s XO_THREADS=%s: %v\n%s", what, threads, err, out)
				}
				return string(out)
			}
			if s := run("sleep", "4"); !strings.Contains(s, "sleep: early 0 of 80") {
				t.Errorf("C sleeps cut short by preemption (XO_THREADS=4):\n%s", s)
			}
			for _, n := range []string{"1", "2"} {
				if s := run("wait", n); !strings.Contains(s, "wait: 6 of 6 saw the flag") {
					t.Errorf("tasks blocked in C starved the setter (XO_THREADS=%s):\n%s", n, s)
				}
			}
		})
	}
}
