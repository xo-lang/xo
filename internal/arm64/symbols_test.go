package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// runFault builds and runs the traceback test program, returning its
// stderr; it must fault (exit 70).
func runFault(t *testing.T, bin string) string {
	t.Helper()
	cmd := testtmp.Guard(exec.Command(bin))
	cmd.Env = append(os.Environ(), "XO_TRACEBACK=1")
	var errb strings.Builder
	cmd.Stderr = &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 70 {
		t.Fatalf("want exit 70, got %v\n%s", err, errb.String())
	}
	return errb.String()
}

// TestReleaseSymbols: a --release link keeps the local function symbols
// (LinkOpts.release; std/native/llrt's prof names a traceback's frames
// from them) and drops the other local symbols. The traceback of a
// release build names its frames, and on darwin the symbol table has no
// LSDA names: before 2026-10-25 the assembler-local labels were symbols,
// 67 of hello's 856 and 559 of CRUD's 2,971.
func TestReleaseSymbols(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) {
			res, err := xotool.Build([]string{filepath.Join("testdata", "traceback")}, xotool.Options{Backend: be, Release: true})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			bin := res.Binaries[xotool.Host()]
			// no DWARF in a release build: frames print their names alone
			if got := runFault(t, bin); !strings.Contains(got, "traceback:\n  down\n") {
				t.Fatalf("a release traceback does not name its frames:\n%s", got)
			}
			if runtime.GOOS != "darwin" {
				return
			}
			nm, err := exec.LookPath("nm")
			if err != nil {
				t.Skip("no nm")
			}
			out, err := exec.Command(nm, bin).Output()
			if err != nil {
				t.Fatalf("nm: %v", err)
			}
			syms := string(out)
			if be == xotool.BackendArm64 {
				if strings.Contains(syms, "GCC_except_table") {
					t.Errorf("an LSDA name in the release symbol table:\n%s", syms)
				}
				if !strings.Contains(syms, "_xo_f_") {
					t.Errorf("no function names in the release symbol table:\n%s", syms)
				}
			} else if !strings.Contains(syms, "down") {
				// the LLVM backend names a function x.<module>.<name>
				t.Errorf("no function names in the release symbol table:\n%s", syms)
			}
		})
	}
}
