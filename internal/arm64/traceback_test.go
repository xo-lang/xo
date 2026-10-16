package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTraceback: with XO_TRACEBACK=1 an uncaught fault's report is
// followed by its frames (decision 0239: fault.xo trace, prof.xo
// trace_text), innermost first, each a function and its .xo line from
// the program's DWARF line tables, down to main; without it the report
// is the one line it always was.
func TestTraceback(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "traceback")}, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	run := func(env ...string) string {
		cmd := exec.Command(bin)
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
	if got := run(); got != report {
		t.Fatalf("without XO_TRACEBACK:\n%s", got)
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
