package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestTraceback: an uncaught fault's report is followed by its frames
// (decision 0239: fault.xo trace, prof.xo trace_text), innermost first,
// each a function and its .xo line from the program's DWARF line
// tables, down to main: by default since 2026-10-22 (owner) and with
// XO_TRACEBACK=1; with XO_TRACEBACK=0 the report is the one line.
func TestTraceback(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) { testTraceback(t, be) })
	}
}

// tracebackBackends: the native backends on this host (arm64, and LLVM
// when clang is there), so CI's linux/arm64 runner checks LLVM's
// tracebacks as well as the Mac's.
func tracebackBackends(t *testing.T) []string {
	bes := []string{xotool.BackendArm64}
	if _, err := xotool.Clang(); err == nil {
		bes = append(bes, xotool.BackendLLVM)
	}
	return bes
}

func testTraceback(t *testing.T, be string) {
	res, err := xotool.Build([]string{filepath.Join("testdata", "traceback")}, xotool.Options{Backend: be})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	run := func(env ...string) string {
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Env = append(os.Environ(), env...)
		var errb strings.Builder
		cmd.Stderr = &errb
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 70 {
			t.Fatalf("want exit 70, got %v\n%s", err, errb.String())
		}
		return errb.String()
	}
	const report = "fault: index 7 out of range for List of length 3 at main.xo:5:12\n"
	if got := run("XO_TRACEBACK=0"); got != report {
		t.Fatalf("with XO_TRACEBACK=0:\n%s", got)
	}
	// on by default (owner, 2026-10-22)
	if got := run(); !strings.HasPrefix(got, report+"traceback:\n") {
		t.Fatalf("without XO_TRACEBACK (the default):\n%s", got)
	}
	got := run("XO_TRACEBACK=1")
	if !strings.HasPrefix(got, report+"traceback:\n") {
		t.Fatalf("with XO_TRACEBACK=1:\n%s", got)
	}
	frames := strings.Split(strings.TrimSuffix(strings.TrimPrefix(got, report+"traceback:\n"), "\n"), "\n")
	// the fault's frame (line 5), three recursive calls (line 7), main (11)
	want := []string{"  down (main.xo:5)", "  down (main.xo:7)", "  down (main.xo:7)", "  down (main.xo:7)", "  main (main.xo:11)"}
	if strings.Join(frames, "\n") != strings.Join(want, "\n") {
		t.Fatalf("frames:\n%s\nwant:\n%s", strings.Join(frames, "\n"), strings.Join(want, "\n"))
	}
	if regexp.MustCompile(`0x[0-9a-f]`).MatchString(got) {
		t.Fatalf("an address in the traceback:\n%s", got)
	}
}

// TestTracebackInlined: frames of inlined calls (the arm64 inliner's
// copies, LLVM's) are in the traceback too, from the DWARF inlined_subroutine entries (decision
// 0239), marked [inlined], innermost first, each at its own line.
func TestTracebackInlined(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) {
			res, err := xotool.Build([]string{filepath.Join("testdata", "traceback_inl")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
			cmd.Env = append(os.Environ(), "XO_TRACEBACK=1")
			var errb strings.Builder
			cmd.Stderr = &errb
			_ = cmd.Run()
			want := "traceback:\n  inner (main.xo:4) [inlined]\n  outer (main.xo:8) [inlined]\n  main (main.xo:15)\n"
			if !strings.HasSuffix(errb.String(), want) {
				t.Fatalf("stderr:\n%s\nwant it to end with:\n%s", errb.String(), want)
			}
		})
	}
}

// TestTracebackContract: a failed `requires` names its function and the
// clause's line, innermost first (field report on 0.26.0, 2026-10-23: the
// top frame was the string helper the failure's detail inlines,
// `prelude.llrt_concat_int (prelude.xo:2703) [inlined]`, then
// `slug (slug.xo:0)`: the check and its failure path were lowered before
// any statement set a line, compiler/ssa contract.xo).
func TestTracebackContract(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) {
			res, err := xotool.Build([]string{filepath.Join("testdata", "traceback_contract")}, xotool.Options{Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
			var errb strings.Builder
			cmd.Stderr = &errb
			err = cmd.Run()
			if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 70 {
				t.Fatalf("want exit 70, got %v\n%s", err, errb.String())
			}
			want := "fault: requires failed: n > 0 at slug.xo:3:12\n   n = 0\ntraceback:\n  slug (slug.xo:3)\n  main (main.xo:10)\n"
			if errb.String() != want {
				t.Fatalf("got:\n%s\nwant:\n%s", errb.String(), want)
			}
		})
	}
}
