package llvm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestRuntimeHeapIdle runs testdata/rt/heapidle (decision 0676): about 64
// MB of small blocks made and freed, then three age pass periods idle;
// the resident set must come back to within 8 MB of its value before the
// burst. Before decision 0676 the small classes' free lists kept the
// whole burst resident (the sql stress program's 56 to 88 MB). Linux
// only: VmRSS.
func TestRuntimeHeapIdle(t *testing.T) {
	if testing.Short() {
		t.Skip("idles for 95 s")
	}
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/self/status")
	}
	dir := filepath.Join("testdata", "rt", "heapidle")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			t.Parallel()
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
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := testtmp.Guard(exec.CommandContext(ctx, res.Binaries[xotool.Host()]))
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			get := func(k string) int {
				m := regexp.MustCompile(`(?m)^` + k + ` (\d+)$`).FindStringSubmatch(string(out))
				if m == nil {
					t.Fatalf("no %q line:\n%s", k, out)
				}
				n, _ := strconv.Atoi(m[1])
				return n
			}
			before, peak, after := get("before"), get("peak"), get("after")
			t.Logf("VmRSS before %d KiB, peak %d KiB, after %d KiB", before, peak, after)
			if peak-before < 32*1024 {
				t.Fatalf("the burst added only %d KiB: it no longer measures the free lists", peak-before)
			}
			if !regexp.MustCompile(`(?m)^reuse ok$`).MatchString(string(out)) {
				t.Errorf("the second burst, on the pages given back, read wrong:\n%s", out)
			}
			if after-before > 8*1024 {
				t.Errorf("after three idle periods VmRSS is %d KiB over its value before the burst (want at most 8192): the free lists kept the burst", after-before)
			}
		})
	}
}

// TestRuntimeHeapIdleLists runs testdata/rt/heapidlelists (task #310):
// sixteen tasks at once free a little under a magazine in each of 64
// prefixed classes, so the blocks stay on many workers' records' free
// lists, then the program idles three age pass periods; the resident set
// must come back to within 8 MB of its value before the burst. Before
// task #310 the sweep read only the central lists and the claiming
// worker's record, and the other records kept the burst (the stress
// programs of std/sse, amqptest, grpcreflect and otlp grew 20 to 120%
// with nothing live). Linux only: VmRSS.
func TestRuntimeHeapIdleLists(t *testing.T) {
	if testing.Short() {
		t.Skip("idles for 95 s")
	}
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/self/status")
	}
	if runtime.NumCPU() < 8 {
		t.Skip("needs 8 workers for the frees to spread over records")
	}
	dir := filepath.Join("testdata", "rt", "heapidlelists")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			t.Parallel()
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
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := testtmp.Guard(exec.CommandContext(ctx, res.Binaries[xotool.Host()]))
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			get := func(k string) int {
				m := regexp.MustCompile(`(?m)^` + k + ` (\d+)$`).FindStringSubmatch(string(out))
				if m == nil {
					t.Fatalf("no %q line:\n%s", k, out)
				}
				n, _ := strconv.Atoi(m[1])
				return n
			}
			before, peak, after := get("before"), get("peak"), get("after")
			t.Logf("VmRSS before %d KiB, peak %d KiB, after %d KiB", before, peak, after)
			if peak-before < 32*1024 {
				t.Fatalf("the burst added only %d KiB: it no longer measures the records' lists", peak-before)
			}
			if !regexp.MustCompile(`(?m)^reuse ok$`).MatchString(string(out)) {
				t.Errorf("the burst after the idle read wrong:\n%s", out)
			}
			if after-before > 8*1024 {
				t.Errorf("after three idle periods VmRSS is %d KiB over its value before the burst (want at most 8192): the records' free lists kept the burst", after-before)
			}
		})
	}
}

// TestRuntimeHeapIdleTHP runs testdata/rt/heapidlethp under XO_THP=dense
// (decision 0676): about 64 MB of 40 KiB and 200 KiB strings made and
// freed, then 150 s idle. The scavenger gives the blocks' pages back
// with MADV_DONTNEED; before it also advised their chunks
// MADV_NOHUGEPAGE, khugepaged refilled them as huge pages within a
// minute (the websocket stress's idle tail on amd64, 2026-11-16: 212 MB
// dense, 96 MB off; this program on linux/arm64 host 2, 2026-11-17: VmRSS
// 38.5 MB after 120 s before the fix, refilled from 100 s on, 2.7 MB
// after). Linux only, and only where the kernel's huge pages
// are not "never".
func TestRuntimeHeapIdleTHP(t *testing.T) {
	if testing.Short() {
		t.Skip("idles for 150 s")
	}
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc/self/status")
	}
	if b, err := os.ReadFile("/sys/kernel/mm/transparent_hugepage/enabled"); err != nil || regexp.MustCompile(`\[never\]`).Match(b) {
		t.Skip("transparent huge pages are off")
	}
	dir := filepath.Join("testdata", "rt", "heapidlethp")
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		be := be
		t.Run(be, func(t *testing.T) {
			if be == xotool.BackendArm64 && runtime.GOARCH != "arm64" {
				t.Skip("the arm64 backend runs on arm64 hosts")
			}
			t.Parallel()
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
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			cmd := testtmp.Guard(exec.CommandContext(ctx, res.Binaries[xotool.Host()], "150"))
			cmd.Env = append(os.Environ(), "XO_THP=dense")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run: %v\n%s", err, out)
			}
			get := func(k string) int {
				m := regexp.MustCompile(`(?m)^` + k + ` (\d+)$`).FindStringSubmatch(string(out))
				if m == nil {
					t.Fatalf("no %q line:\n%s", k, out)
				}
				n, _ := strconv.Atoi(m[1])
				return n
			}
			before, peak, after := get("before"), get("peak"), get("after")
			t.Logf("VmRSS before %d KiB, peak %d KiB, after %d KiB\n%s", before, peak, after, out)
			if peak-before < 8*1024 {
				t.Fatalf("the burst added only %d KiB: it no longer measures the scavenger", peak-before)
			}
			if !regexp.MustCompile(`(?m)^reuse ok$`).MatchString(string(out)) {
				t.Errorf("the second burst, on the pages given back, read wrong:\n%s", out)
			}
			if after-before > thpSlackKiB {
				t.Errorf("after 150 s idle under XO_THP=dense VmRSS is %d KiB over its value before the burst (want at most %d): khugepaged refilled the pages the scavenger gave back", after-before, thpSlackKiB)
			}
		})
	}
}

// thpSlackKiB: the resident set TestRuntimeHeapIdleTHP allows over its
// value before the burst (host 2: 1.5 MB with the fix, 37 MB without).
const thpSlackKiB = 8 * 1024
