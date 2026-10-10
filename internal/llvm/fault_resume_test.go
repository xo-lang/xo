package llvm_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
)

// TestRuntimeFaultResume raises a fault through a C frame whose cleanup
// resumes the unwind and scribbles over the stack the raising frames
// used (std/llrt/testdata/fault_resume.c). The fault's exception object
// must outlive those frames: the cleanup runs, the fault is reported past
// the boundary, and the process exits 70. Built plainly and with
// ThreadSanitizer, whose instrumentation adds such a cleanup to every
// function (the Linux CI failure of 2026-10-09).
func TestRuntimeFaultResume(t *testing.T) {
	t.Parallel() // builds nothing in the shared build directory
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found")
	}
	src := filepath.Join(rtDir, "testdata", "fault_resume.c")
	plain := filepath.Join(t.TempDir(), "plain")
	if err := build.CompileC([]string{"-O1", "-g"}, []string{src}, plain); err != nil {
		t.Fatal(err)
	}
	bins := map[string]string{"plain": plain, "tsan": tsanBuild(t, nil, []string{src})}
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
		for _, want := range []string{"cleanup ran\n", "fault: boom at fault_resume.xo:3:5\n"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: output lacks %q:\n%s", name, want, text)
			}
		}
		if strings.Contains(text, "unreachable") || strings.Contains(text, "ThreadSanitizer") {
			t.Errorf("%s: unexpected output:\n%s", name, text)
		}
	}
}
