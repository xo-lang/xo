package llvm_test

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
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			dir := filepath.Join("testdata", "realtime", "alloc")
			r, b := buildRun(t, dir, xotool.Options{Backend: be})
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if r.String() != string(want) {
				t.Errorf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, r)
			}
			leakCheck(t, b.Binaries[xotool.Host()], dir)

			dir = filepath.Join("testdata", "golden", "realtime_section")
			_, b = buildRun(t, dir, xotool.Options{Backend: be})
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
			res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
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

// TestRuntimeRealtime runs the C test of real-time sections (decision
// 0133): deferred frees, nesting, queue overflow, counted allocations,
// and sections on several threads, under ThreadSanitizer and then
// optimized; every allocation must be freed.
func TestRuntimeRealtime(t *testing.T) {
	t.Parallel()
	src := filepath.Join(rtDir, "testdata", "realtime.c")
	bin := tsanBuild(t, nil, cOnly(t, src))
	out := runTSan(t, bin, ".")
	for _, name := range []string{"defer", "nested", "overflow", "allocs", "threads"} {
		if !strings.Contains(out, name+": ok") {
			t.Errorf("test %s did not pass:\n%s", name, out)
		}
	}
	if testing.Short() {
		return
	}
	opt := exe(filepath.Join(t.TempDir(), "opt"))
	if err := xotool.CompileC([]string{"-O2"}, cOnly(t, src), opt); err != nil {
		t.Fatal(err)
	}
	runTSan(t, opt, ".")
}
