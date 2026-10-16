package llvm

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

var rtStatsRE = regexp.MustCompile(`rt_section=(\d+) rt_defer=(\d+) rt_overflow=(\d+)`)

// TestRealtimeNative checks the native side of real-time sections
// (decision 0133) on LLVM and, on arm64 hosts, arm64:
//
//   - testdata/realtime/alloc: allocations the checker cannot see (a
//     shared List copied on write, a recursive enum's box, a defaulted
//     List field) fault with XO-F007 at the end of the section, which
//     task.isolate catches, and nothing leaks (the Go backend has no
//     sections, so this program is not a golden);
//   - the realtime_section golden: frees are deferred inside sections
//     (XO_RTSTATS counts them) and none overflows the queue.
func TestRealtimeNative(t *testing.T) {
	t.Parallel()
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			t.Parallel()
			// its own build directory: TestGolden builds realtime_section
			// at the same time, and two builds of one program into the
			// shared directory (TestMain's XO_BUILD_DIR) would race
			opts := xotool.Options{Backend: be, Env: []string{"XO_BUILD_DIR=" + filepath.Join(t.TempDir(), "build")}}
			dir := filepath.Join("testdata", "realtime", "alloc")
			r, b := buildRun(t, dir, opts)
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if r.String() != string(want) {
				t.Errorf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, r)
			}
			leakCheck(t, b.Binaries[xotool.Host()], dir)

			dir = filepath.Join("testdata", "golden", "realtime_section")
			_, b = buildRun(t, dir, opts)
			cmd := exec.Command(b.Binaries[xotool.Host()])
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "XO_RTSTATS=1")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("run: %v\n%s", err, stderr.String())
			}
			m := rtStatsRE.FindStringSubmatch(stderr.String())
			if m == nil {
				t.Fatalf("no rt_section counts in XO_RTSTATS output:\n%s", stderr.String())
			}
			sections, _ := strconv.Atoi(m[1])
			deferred, _ := strconv.Atoi(m[2])
			overflow, _ := strconv.Atoi(m[3])
			if sections < 20 || deferred < 3 || overflow != 0 {
				t.Errorf("rt_section=%d rt_defer=%d rt_overflow=%d: want at least 20 sections, 3 deferred frees, no overflow", sections, deferred, overflow)
			}

			// the demo (bench/results/realtime-2026-10-10.md), each tick
			// version briefly, every allocation freed
			dir = filepath.Join("..", "..", "examples", "realtime-control")
			res, err := xotool.Build([]string{dir}, opts)
			if err != nil || res.Failed {
				t.Fatalf("build the demo: %v %v", err, res.Output)
			}
			for _, mode := range []string{"rt", "plain", "alloc"} {
				cmd := exec.Command(res.Binaries[xotool.Host()], mode, "3000", "500")
				cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
				var out bytes.Buffer
				cmd.Stdout, cmd.Stderr = &out, &out
				if err := cmd.Run(); err != nil {
					t.Fatalf("demo %s: %v\n%s", mode, err, out.String())
				}
				for _, want := range []string{mode + " iterations 3000 swaps 8 checksum ", "swap output min ", "xo leakcheck: 0 live"} {
					if !strings.Contains(out.String(), want) {
						t.Errorf("demo %s: no %q in\n%s", mode, want, out.String())
					}
				}
			}
		})
	}
}

// TestRuntimeRealtime runs the test of real-time sections (decision
// 0133), an Xo program since decision 0241 (testdata/rt/realtime; it was
// std/llrt/testdata/realtime.c): deferred frees, a real-time call inside a
// section, queue overflow, a counted allocation and its XO-F007, and
// sections on several threads at once, under ThreadSanitizer with one
// worker, three, and eight and then optimized; every allocation must be
// freed.
func TestRuntimeRealtime(t *testing.T) {
	t.Parallel()
	runRuntimeXo(t, filepath.Join("testdata", "rt", "realtime"), "",
		[]string{"defer", "nested", "overflow", "allocs", "threads"})
}
