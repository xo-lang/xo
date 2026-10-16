package llvm

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestDeferredWakeBound: a wake the scheduler defers (decision 0077's
// addendum: a channel's hand off, a ring's only spawn; mutexbusy checks a
// Mutex's grant, which main wakes directly for now)
// still runs its task while the waker keeps computing and never parks,
// because the monitor wakes an idle worker for it after about 1 ms.
// testdata/sched handoff (a sender), spawnbusy (a spawner), and mutexbusy
// (a Mutex's holder) compute 200 ms after the wake and print the worst
// delay of 5 rounds at XO_THREADS=4; it must stay under 100 ms. Without
// the monitor the task waits the whole 200 ms; with it, about 2 ms on an
// idle machine, but its 1 ms ticks overslept by tens of ms on a Mac at
// load 25 (2026-10-16), so the bound tells broken from working, not fast
// from slow.
func TestDeferredWakeBound(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs two programs per native backend")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" {
		backends = append(backends, xotool.BackendArm64)
	}
	worst := regexp.MustCompile(`worst (\d+) us \(busy\)`)
	for _, be := range backends {
		for _, prog := range []string{"handoff", "spawnbusy", "mutexbusy"} {
			t.Run(be+"/"+prog, func(t *testing.T) {
				if be == xotool.BackendLLVM {
					if _, err := xotool.Clang(); err != nil {
						t.Skip("clang not found")
					}
				}
				res, err := xotool.Build([]string{filepath.Join("testdata", "sched", prog)}, xotool.Options{Backend: be})
				if err != nil || res.Failed {
					t.Fatalf("build: %v %v", err, res)
				}
				cmd := exec.Command(res.Binaries[xotool.Host()])
				cmd.Env = append(os.Environ(), "XO_THREADS=4")
				out, err := runCombined(cmd, 2*time.Minute)
				if err != nil {
					t.Fatalf("run: %v\n%s", err, out)
				}
				m := worst.FindStringSubmatch(out)
				if m == nil {
					t.Fatalf("output: %s", out)
				}
				us, _ := strconv.Atoi(m[1])
				if us >= 100000 {
					t.Fatalf("%s: the woken task waited %d us (bound 100 ms; the waker computed 200 ms)", prog, us)
				}
				t.Logf("%s: worst %d us", prog, us)
			})
		}
	}
}
