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

// TestRuntimeSleepClock runs testdata/rt/sleepclock on both native
// backends (task #95): poll.sleep_until on the timers' clock returns
// TIMEOUT, and a deadline on the wall clock faults naming the clock, where
// it parked the task for about 56 years (the run hit this test's timeout).
func TestRuntimeSleepClock(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program twice")
	}
	dir := filepath.Join("testdata", "rt", "sleepclock")
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
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := testtmp.Guard(exec.CommandContext(ctx, res.Binaries[xotool.Host()]))
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("still parked after 20 s (a wall clock deadline):\n%s", out)
			}
			if err == nil {
				t.Fatalf("exit 0, want a fault:\n%s", out)
			}
			s := string(out)
			if !strings.Contains(s, "mono: 3\n") {
				t.Errorf("no \"mono: 3\":\n%s", s)
			}
			if strings.Contains(s, "wall returned") {
				t.Errorf("the wall clock deadline returned:\n%s", s)
			}
			if !strings.Contains(s, "past the timers' clock") {
				t.Errorf("no fault naming the clock:\n%s", s)
			}
		})
	}
}
