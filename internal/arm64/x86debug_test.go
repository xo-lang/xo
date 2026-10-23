package arm64

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
	for _, name := range []string{"http_server", "fault_unwind", "float_libm", "json_text"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			ins := func(flags ...string) []string {
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
				var lines []string
				for _, l := range strings.Split(string(text), "\n") {
					if strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, "\t.") {
						lines = append(lines, l)
					}
				}
				return lines
			}
			with := ins()
			without := ins("-s")
			if len(with) != len(without) {
				t.Fatalf("%d instructions with debug information, %d without", len(with), len(without))
			}
			for i := range with {
				if with[i] != without[i] {
					t.Fatalf("instruction %d differs with debug information:\n  with:    %s\n  without: %s", i, with[i], without[i])
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
