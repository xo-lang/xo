package llvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

var faultShared struct {
	once sync.Once
	bins map[string]string
	dir  string
}

// faultSharedBins builds testdata/rt/fault_shared (decision 0241: it was
// std/llrt/testdata/fault_shared.c and fault_resume.c) with each native
// backend and, its own functions marked, with ThreadSanitizer: once for
// both tests (a failed build fails the first, and the second sees no
// binaries).
func faultSharedBins(t *testing.T) map[string]string {
	t.Helper()
	if runtime.GOARCH != "arm64" || runtime.GOOS == "windows" {
		t.Skip("the Xo unwinder runs on arm64 Linux and macOS")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	faultShared.once.Do(func() {
		dir, err := os.MkdirTemp("", "xo-fault-shared")
		if err != nil {
			t.Fatal(err)
		}
		faultShared.dir = dir
		faultShared.bins = buildFaultShared(t, dir)
	})
	if len(faultShared.bins) == 0 {
		t.Fatal("testdata/rt/fault_shared did not build")
	}
	return faultShared.bins
}

func buildFaultShared(t *testing.T, tmp string) map[string]string {
	dir := filepath.Join("testdata", "rt", "fault_shared")
	bins := map[string]string{}
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		res, err := xotool.Build([]string{dir}, xotool.Options{Backend: be})
		if err != nil || res.Failed {
			t.Fatalf("build %s: %v %v", be, err, res)
		}
		bins[be] = res.Binaries[xotool.Host()]
	}
	ir, err := xotool.LLVMIR(dir)
	if err != nil {
		t.Fatal(err)
	}
	ll := filepath.Join(tmp, "prog.ll")
	if err := os.WriteFile(ll, markTSan(t, dir, ir), 0o644); err != nil {
		t.Fatal(err)
	}
	// tsanBuild's binary is in this test's temporary directory: kept
	// beside the IR for the other test
	tb, err := os.ReadFile(tsanBuild(t, []string{"-Wno-override-module"}, []string{ll}))
	if err != nil {
		t.Fatal(err)
	}
	bins["tsan"] = exe(filepath.Join(tmp, "tsan"))
	if err := os.WriteFile(bins["tsan"], tb, 0o755); err != nil {
		t.Fatal(err)
	}
	return bins
}

// TestRuntimeFaultShared raises a fault in a callback of the C library
// (std/internal's rt_probe "qsort_fault": qsort's comparator faults), so
// frames of the C library, a shared object, sit between the fault and
// task.isolate's boundary. The Xo unwinder (decision 0221) must find
// their unwind tables (ld.so's link_map on Linux, dladdr on macOS) to
// reach it: "caught: boom in qsort" and exit status 0.
func TestRuntimeFaultShared(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	for name, bin := range faultSharedBins(t) {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), tsanOptions())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v, want exit status 0:\n%s", name, err, out)
			continue
		}
		if !strings.Contains(string(out), "caught: boom in qsort\n") {
			t.Errorf("%s: output lacks the isolated fault:\n%s", name, out)
		}
	}
}

// TestRuntimeFaultResume raises the same fault without isolating it: it
// unwinds through the C library's frames, which are not generated code
// (the Xo unwinder passes over a frame whose personality is not
// xo_x_personality), and is reported past the boundary with exit status
// 70. fault_resume.c also checked that a C frame's cleanup runs and
// resumes the unwind where libgcc or libunwind unwinds (x86_64), with
// the fault's exception object outliving the raising frames
// (gotchas/tsan-resumes-fault-unwind.md); an Xo program has no C frame
// with a cleanup, so that case is not checked (decision 0241).
func TestRuntimeFaultResume(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	for name, bin := range faultSharedBins(t) {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), tsanOptions(), "XO_FAULT_UNISOLATED=1")
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 70 {
			t.Errorf("%s: want exit status 70, got %v:\n%s", name, err, out)
			continue
		}
		text := string(out)
		if !strings.Contains(text, "fault: boom in qsort at ") {
			t.Errorf("%s: output lacks the fault's report:\n%s", name, text)
		}
		if strings.Contains(text, "unreachable") || strings.Contains(text, "ThreadSanitizer") {
			t.Errorf("%s: unexpected output:\n%s", name, text)
		}
	}
}
