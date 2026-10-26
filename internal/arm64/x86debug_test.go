package arm64

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestAmd64DebugInfoSameCode: the amd64 backend's instructions are the
// same with debug information (line rows, and since 2026-10-20 the
// variables' location labels, dwvars.xo) as without (-s): the labels are
// only labels, and no text pass may treat one as a boundary (the arm64
// backend's TestDebugInfoSameCode compares otool's text; this compares
// the assembly's instruction lines of a linux/amd64 cross build).
func TestAmd64DebugInfoSameCode(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds goldens twice")
	}
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	local := regexp.MustCompile(`\.L[A-Za-z_][A-Za-z0-9_]*`)
	digits := regexp.MustCompile(`^[0-9]+:$`)
	for _, name := range []string{"http_server", "fault_unwind", "float_libm", "json_text"} {
		t.Run(name, func(t *testing.T) {
			// independent cross builds (lead, 2026-10-26: serial, 8 builds)
			t.Parallel()
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			ins := func(flags ...string) map[string][]string {
				out := filepath.Join(t.TempDir(), "a.s")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				args := append(append(append([]string{}, cli[1:]...), "build", "--backend=amd64", "--targets=linux/amd64", "-o", filepath.Join(t.TempDir(), "b"), "--asm="+out), flags...)
				args = append(args, ".")
				cmd := testtmp.Guard(exec.CommandContext(ctx, cli[0], args...))
				cmd.Dir = dir
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
				}
				text, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				// each function's instructions by its label; the build
				// with debug information may hold more functions (the
				// runtime's DWARF readers, dwline.built(), decision 0354's
				// addendum)
				fns := map[string][]string{}
				cur := ""
				for _, l := range strings.Split(string(text), "\n") {
					switch {
					case strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "\t."):
						// a local label's name carries the function's number
						// and a hash of the part: not part of the code
						fns[cur] = append(fns[cur], local.ReplaceAllString(l, ".L"))
					case !strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, ".") && strings.HasSuffix(l, ":") && !digits.MatchString(l):
						cur = strings.TrimSuffix(l, ":")
					}
				}
				return fns
			}
			with := ins()
			without := ins("-s")
			if len(without) == 0 {
				t.Fatalf("no functions found in the assembly")
			}
			for name, want := range without {
				// the runtime's functions that call the DWARF readers differ
				// by the branch on dwline.built() (decision 0354's addendum)
				if !programFunc(name) {
					continue
				}
				got, ok := with[name]
				if !ok {
					t.Fatalf("%s is in the build without debug information, not in the one with it", name)
				}
				if len(got) != len(want) {
					t.Fatalf("%s: %d instructions with debug information, %d without", name, len(got), len(want))
				}
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("%s: instruction %d differs with debug information:\n  with:    %s\n  without: %s", name, i, got[i], want[i])
					}
				}
			}
		})
	}
}

// TestAmd64CrossBuilds: programs whose amd64 builds crashed the backend
// build for every amd64 target (2026-10-23: mir.xo msuccs named a call's
// pad block that the layout had dropped, and shrink's liveness indexed it:
// "index -1 out of range" at arm64/regalloc.xo for bench/perf/portable,
// sql_conv, sql_wire_names, xml_basic on linux, windows and darwin).
func TestAmd64CrossBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds four programs for three targets")
	}
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("..", "llvm", "testdata", "golden")
	dirs := map[string]string{
		"portable":       filepath.Join("..", "..", "bench", "perf", "portable"),
		"sql_conv":       filepath.Join(golden, "sql_conv"),
		"sql_wire_names": filepath.Join(golden, "sql_wire_names"),
		"xml_basic":      filepath.Join(golden, "xml_basic"),
	}
	for name, dir := range dirs {
		for _, target := range []string{"linux/amd64", "windows/amd64", "darwin/amd64"} {
			t.Run(name+"/"+strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
				// independent cross builds (lead, 2026-10-26: serial, 12 builds)
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				args := append(append([]string{}, cli[1:]...), "build", "--backend=amd64", "--targets="+target, "-o", filepath.Join(t.TempDir(), "b"), ".")
				cmd := testtmp.Guard(exec.CommandContext(ctx, cli[0], args...))
				cmd.Dir = dir
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
				}
			})
		}
	}
}
