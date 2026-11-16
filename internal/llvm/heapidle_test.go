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
