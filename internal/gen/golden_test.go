package gen_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
)

var updateGolden = flag.Bool("golden-update", false, "rewrite want.txt of the golden programs")

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "xo-gen-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	os.Setenv("XO_CRASH_DIR", filepath.Join(dir, "crash"))
	os.MkdirAll(filepath.Join(dir, "crash"), 0o755)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runResult is the observable behavior of a program run.
type runResult struct {
	exit   int
	stdout string
	stderr string
}

func (r runResult) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.exit, r.stdout, r.stderr)
}

var timeRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)

// stableStderr keeps the lines of stderr that do not depend on the
// machine: Go stack frames and crash bundle paths are dropped; Xo error
// frames ("  at ...") are kept; wall clock times become <time>.
func stableStderr(s string) string {
	s = timeRE.ReplaceAllString(s, "<time>")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "  at ") {
			out = append(out, l)
			continue
		}
		if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "crash bundle:") {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// goFrameRE matches one Go frame of a fault's stack (function name, then
// its .xo position), as printed by the test runner and crash reports.
var goFrameRE = regexp.MustCompile(`(?m)^ +x_[^ \n]+\n +[^ \n]+\.xo:\d+\n`)

// dropGoFrames removes the Go frame lists of fault stacks. With and
// without --no-mono they legitimately differ: specialized copies have their own Go names
// (`F_max__int64`) and inlined closure literals have no frame. Fault
// messages, locations, and Xo error frames ("  at ...") are kept.
func dropGoFrames(s string) string { return goFrameRE.ReplaceAllString(s, "") }

// buildAndRun builds the program in dir and runs it with the arguments in
// args.txt and the input in stdin.txt.
func buildAndRun(t *testing.T, dir string, mode gen.Mode, extra ...string) (runResult, *build.Result) {
	t.Helper()
	return buildAndRunOpts(t, dir, build.Options{Mode: mode}, extra...)
}

// buildAndRunOpts is buildAndRun with full build options.
func buildAndRunOpts(t *testing.T, dir string, opts build.Options, extra ...string) (runResult, *build.Result) {
	t.Helper()
	res, err := build.Build([]string{dir}, opts)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Failed {
		var b strings.Builder
		for _, d := range res.Diagnostics {
			b.WriteString(diag.Human(d) + "\n")
		}
		t.Fatalf("check failed:\n%s", b.String())
	}
	var args []string
	if data, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = strings.Fields(string(data))
	}
	args = append(args, extra...)
	cmd := exec.Command(res.Binaries[build.Host()], args...)
	cmd.Dir = dir
	if data, err := os.ReadFile(filepath.Join(dir, "stdin.txt")); err == nil {
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return runResult{exit: code, stdout: stdout.String(), stderr: stderr.String()}, res
}

// TestGolden builds and runs every program under testdata/golden with the
// default lowering (specialized generics, inlined closure literals) and
// compares exit status, stdout, and the stable lines of stderr with
// want.txt, apart from the Go frame lists of fault stacks (dropGoFrames).
// Programs with test blocks are run with `xo test` semantics when the
// directory name starts with "test_".
func TestGolden(t *testing.T) {
	if *updateGolden {
		t.Skip("want.txt is written by TestGoldenNoMono")
	}
	testGolden(t, false, false)
}

// TestGoldenNoMono builds every golden program with `--no-mono` (one Go
// generic function per Xo generic function) and compares with want.txt
// exactly, Go frames included; -golden-update writes want.txt from this
// lowering. The two lowerings must not differ in behavior.
func TestGoldenNoMono(t *testing.T) { testGolden(t, true, false) }

// TestGoldenArenas builds every golden program with `--arenas` (request
// scoped arenas, std/rt/arena.go) and compares with want.txt like
// TestGolden: arenas must not change behavior. arena_escape checks that
// values escaping a request through a Mutex, a channel, the response, and
// a background task survive arena reuse.
func TestGoldenArenas(t *testing.T) {
	if *updateGolden {
		t.Skip("want.txt is written by TestGoldenNoMono")
	}
	testGolden(t, false, true)
}

func testGolden(t *testing.T, noMono, arenas bool) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range dirs {
		dir := dir
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			mode := gen.Build
			if strings.HasPrefix(filepath.Base(dir), "test_") {
				mode = gen.Test
			}
			got, _ := buildAndRunOpts(t, dir, build.Options{Mode: mode, NoMono: noMono, Arenas: arenas})
			got.stderr = stableStderr(got.stderr)
			wantPath := filepath.Join(dir, "want.txt")
			if *updateGolden && noMono {
				if err := os.WriteFile(wantPath, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("missing want.txt (run with -golden-update): %v", err)
			}
			gs, ws := got.String(), string(want)
			if !noMono {
				gs, ws = dropGoFrames(gs), dropGoFrames(ws)
			}
			if gs != ws {
				t.Errorf("output differs\n--- got\n%s\n--- want\n%s", got.String(), want)
			}
		})
	}
}
