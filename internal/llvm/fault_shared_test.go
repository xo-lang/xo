package llvm

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestRuntimeFaultShared raises a fault in a qsort comparator
// (std/llrt/testdata/fault_shared.c), so frames of the C library, a
// shared object, sit between the fault and the landing pad of
// fault_shared.ll's catcher. The Xo unwinder (decision 0221) must find
// their unwind tables (ld.so's link_map on Linux, dladdr on macOS) to
// reach the catcher: "caught" and exit status 0. arm64 Linux and macOS
// only: x86_64 still unwinds with the C personality, which this
// catcher does not name.
func TestRuntimeFaultShared(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if runtime.GOARCH != "arm64" || runtime.GOOS == "windows" {
		t.Skip("the Xo unwinder runs on arm64 Linux and macOS")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	srcs := append(cOnly(t, filepath.Join(rtDir, "testdata", "fault_shared.c")), filepath.Join(rtDir, "testdata", "fault_shared.ll"))
	bin := exe(filepath.Join(t.TempDir(), "shared"))
	if err := xotool.CompileC([]string{"-O1", "-g", "-Wno-override-module"}, srcs, bin); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin).CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		t.Fatalf("exit status %d, want 0:\n%s", ee.ExitCode(), out)
	} else if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "caught\n") {
		t.Errorf("output lacks \"caught\":\n%s", out)
	}
}
