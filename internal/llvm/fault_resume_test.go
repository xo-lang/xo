package llvm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestRuntimeFaultResume raises a fault through a C frame with a
// cleanup (std/llrt/testdata/fault_resume.c). The fault is reported past
// the boundary and the process exits 70. Where libgcc or libunwind runs
// the unwind (x86_64) the C cleanup runs and resumes the unwind after
// scribbling over the stack the raising frames used, so the fault's
// exception object must outlive those frames
// (gotchas/tsan-resumes-fault-unwind.md); where the Xo unwinder runs it
// (arm64 Linux and macOS, decision 0221 stage 3) a frame whose
// personality is not xo_x_personality is passed over and its cleanup
// does not run. Built
// plainly and with ThreadSanitizer.
func TestRuntimeFaultResume(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "fault_resume.c")
	plain := exe(filepath.Join(t.TempDir(), "plain"))
	if err := xotool.CompileC([]string{"-O1", "-g"}, cOnly(t, src), plain); err != nil {
		t.Fatal(err)
	}
	bins := map[string]string{"plain": plain, "tsan": tsanBuild(t, nil, cOnly(t, src))}
	for name, bin := range bins {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), tsanOptions())
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 70 {
			t.Errorf("%s: want exit status 70, got %v:\n%s", name, err, out)
			continue
		}
		text := string(out)
		if !strings.Contains(text, "fault: boom at fault_resume.xo:3:5\n") {
			t.Errorf("%s: output lacks the fault's report:\n%s", name, text)
		}
		if ran := strings.Contains(text, "cleanup ran\n"); ran != cCleanupsRun() {
			t.Errorf("%s: the C cleanup ran: %v, want %v:\n%s", name, ran, cCleanupsRun(), text)
		}
		if strings.Contains(text, "unreachable") || strings.Contains(text, "ThreadSanitizer") {
			t.Errorf("%s: unexpected output:\n%s", name, text)
		}
	}
}

// cCleanupsRun reports whether a fault runs the cleanups of C frames it
// unwinds: not where the Xo unwinder walks the frames (arm64 Linux and
// macOS, decision 0221 stage 3), which enters only generated code's
// landing pads.
func cCleanupsRun() bool {
	return runtime.GOOS == "windows" || runtime.GOARCH != "arm64"
}
