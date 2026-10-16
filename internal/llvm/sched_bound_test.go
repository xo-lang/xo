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
// addendum: a channel's hand off, a ring's only spawn) still runs its
// task soon when the waker keeps computing and never parks, because the
// monitor wakes an idle worker for it after about 1 ms. testdata/sched
// handoff (a sender that computes 30 ms after its send) and spawnbusy (a
// spawner that computes 30 ms after its spawn) print the worst delay of
// 20 rounds at XO_THREADS=4; it must stay under 10 ms (about 2 ms
// expected; the margin is for a loaded machine).
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
		for _, prog := range []string{"handoff", "spawnbusy"} {
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
				if us >= 10000 {
					t.Fatalf("%s: the woken task waited %d us (bound 10 ms)", prog, us)
				}
				t.Logf("%s: worst %d us", prog, us)
			})
		}
	}
}
