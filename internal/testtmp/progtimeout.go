package testtmp

import (
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// DefaultProgTimeout is how long RunTimed lets a test program run.
const DefaultProgTimeout = 2 * time.Minute

// ProgTimeout is how long a golden or test program may run before
// RunTimed stops it: XO_TEST_PROG_TIMEOUT (a Go duration, such as 5m), else
// DefaultProgTimeout. A hung program then fails its own test with its
// name instead of holding the package to go test's -timeout (net_close_flush
// held tier 48's LLVM step about 40 minutes, 2026-11-04).
func ProgTimeout() time.Duration {
	if v := os.Getenv("XO_TEST_PROG_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultProgTimeout
}

// RunTimed runs cmd as cmd.Run does, stopping it after ProgTimeout: the
// program (and, behind Guard's lifeline wrapper, its whole process group)
// gets SIGKILL, and timedOut is true.
func RunTimed(cmd *exec.Cmd) (timedOut bool, err error) {
	if cmd.WaitDelay == 0 {
		// a child it started that keeps stdout open does not hold Wait
		cmd.WaitDelay = 10 * time.Second
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}
	var fired atomic.Bool
	p := cmd.Process
	t := time.AfterFunc(ProgTimeout(), func() {
		fired.Store(true)
		killTree(p)
	})
	err = cmd.Wait()
	t.Stop()
	return fired.Load(), err
}
