package llvm_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/llvm"
	"github.com/xo-lang/xo/internal/testtmp"
)

var updateGolden = flag.Bool("golden-update", false, "rewrite want.txt of the golden programs")

// testDir is the temporary directory TestMain makes and removes.
var testDir string

func TestMain(m *testing.M) {
	flag.Parse()
	// identical builds reuse one binary, within and across runs
	build.EnableTestBinCache()
	dir, err := testtmp.Dir("xo-llvm-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testDir = dir
	// the http-diff driver used its own directory before it moved into testDir
	testtmp.Sweep("xo-http-diff-")
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
// backend or the machine, as internal/gen's golden test does: the Go
// backend's stack frames (indented lines) and crash bundle paths are
// dropped; fault messages with their positions are kept.
func stableStderr(s string) string {
	s = timeRE.ReplaceAllString(s, "<time>")
	var out []string
	for _, l := range strings.Split(s, "\n") {
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

func buildAndRun(t *testing.T, dir string, opts build.Options) runResult {
	t.Helper()
	r, _ := buildRun(t, dir, opts)
	return r
}

// buildRun builds and runs the program in dir and also returns the build
// result.
func buildRun(t *testing.T, dir string, opts build.Options) (runResult, *build.Result) {
	t.Helper()
	res, err := build.Build([]string{dir}, opts)
	if err != nil {
		t.Fatalf("build (%s): %v", opts.Backend, err)
	}
	if res.Failed {
		var b strings.Builder
		for _, d := range res.Diagnostics {
			b.WriteString(diag.Human(d) + "\n")
		}
		t.Fatalf("check failed:\n%s", b.String())
	}
	cmd := exec.Command(res.Binaries[build.Host()], progArgs(dir)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), progEnv(dir)...)
	cmd.Stdin = progStdin(dir)
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
	return runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(stderr.String())}, res
}

// leakCheck runs bin with XO_LEAKCHECK=1 and requires every allocation
// freed at exit.
func leakCheck(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := exec.Command(bin, progArgs(dir)...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), progEnv(dir)...), "XO_LEAKCHECK=1")
	cmd.Stdin = progStdin(dir)
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("leak check run: %v\n%s", err, stderr.String())
	}
	m := leakRE.FindStringSubmatch(stderr.String())
	if m == nil {
		t.Fatalf("no leak check line in stderr:\n%s", stderr.String())
	}
	if m[1] != "0" {
		t.Errorf("%s allocations still live at exit (of %s)", m[1], m[2])
	}
	return m[0]
}

// progArgs are the command line arguments of the program in dir: the
// fields of its args.txt, as internal/gen's golden test passes them.
func progArgs(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

// progEnv are the environment variables a golden program in dir runs
// with besides the test's: the NAME=VALUE lines of its env.txt.
func progEnv(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

// progStdin is the standard input of a golden program in dir: its
// stdin.txt, or nothing.
func progStdin(dir string) io.Reader {
	data, err := os.ReadFile(filepath.Join(dir, "stdin.txt"))
	if err != nil {
		return nil
	}
	return bytes.NewReader(data)
}

var leakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// TestGolden builds every program under testdata/golden with the Go
// backend and with the LLVM backend and requires the same exit status,
// stdout, and fault messages, which must also match want.txt. Programs
// that exit normally run again with XO_LEAKCHECK=1 and must free every
// allocation.
// shortGoldens are the golden programs TestGolden runs under -short too
// (about a second each): native parity checked on every commit for the
// cases with no Go backend golden of their own.
var shortGoldens = map[string]bool{"fault_defer_chain": true, "request_eq": true}

func TestGolden(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			if testing.Short() && !shortGoldens[filepath.Base(dir)] {
				t.Skip("builds every golden program twice: -short runs shortGoldens")
			}
			t.Parallel()
			goRes := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendGo})
			llRes, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM})
			if goRes.String() != llRes.String() {
				t.Errorf("LLVM backend differs from the Go backend\n--- go\n%s\n--- llvm\n%s", goRes, llRes)
			}
			if llRes.exit == 0 {
				leakCheck(t, b.Binaries[build.Host()], dir)
			}
			wantPath := filepath.Join(dir, "want.txt")
			if *updateGolden {
				if err := os.WriteFile(wantPath, []byte(goRes.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("missing want.txt (run with -golden-update): %v", err)
			}
			if string(want) != llRes.String() {
				t.Errorf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, llRes)
			}
		})
	}
}

// TestGoldenShared builds every golden program with share probes (every
// heap value marked shared between threads as it is defined, decision
// 0058) and requires want.txt and no allocation live at exit: the atomic
// count paths, the shared claim of in place updates, and the share
// walkers of every type must behave like the thread local ones.
func TestGoldenShared(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds every golden program")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			got, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM, ShareProbes: true})
			if got.exit == 0 {
				leakCheck(t, b.Binaries[build.Host()], dir)
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got.String() {
				t.Errorf("output differs from want.txt\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}

// TestUnsupported checks that constructs outside the subset are rejected
// with an error naming the construct and its position. Each program's
// want.txt lists substrings the error must contain, one per line.
func TestUnsupported(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "unsupported", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			res, ok, err := build.CheckNative([]string{dir}, false)
			if err != nil || !ok {
				var b strings.Builder
				if res != nil {
					for _, d := range res.Diagnostics {
						b.WriteString(diag.Human(d) + "\n")
					}
				}
				t.Fatalf("check failed: %v\n%s", err, b.String())
			}
			abs, _ := filepath.Abs(dir)
			_, err = llvm.Generate(res, llvm.Options{LineRoot: abs})
			if err == nil {
				t.Fatal("generation succeeded; want an unsupported construct error")
			}
			want, err2 := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err2 != nil {
				t.Fatal(err2)
			}
			for _, w := range strings.Split(strings.TrimSpace(string(want)), "\n") {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error does not contain %q:\n%v", w, err)
				}
			}
		})
	}
}

// TestBuildErrors covers the options the LLVM backend rejects.
func TestBuildErrors(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "arith")
	cases := []struct {
		opts build.Options
		want string
	}{
		{build.Options{Mode: gen.Build, Backend: build.BackendLLVM, Targets: []build.Target{{OS: "plan9", Arch: "amd64"}}}, "host"},
		{build.Options{Mode: gen.Build, Backend: "wasm"}, "unknown backend"},
	}
	for _, c := range cases {
		_, err := build.Build([]string{dir}, c.opts)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want an error containing %q", c.opts, err, c.want)
		}
	}
}

// TestLeakCheck builds each benchmark program (bench/perf/llvm/xo) with
// the LLVM backend and requires the expected output and that
// every allocation is freed by exit (the runtime's allocation counter,
// XO_LEAKCHECK=1).
func TestLeakCheck(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("runs the benchmark programs")
	}
	dirs, err := filepath.Glob(filepath.Join("..", "..", "bench", "perf", "llvm", "xo", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no bench programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			r, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM})
			if r.exit != 0 || !strings.HasPrefix(r.stdout, filepath.Base(dir)+" ") {
				t.Fatalf("run: %s", r)
			}
			t.Log(strings.TrimSpace(r.stdout), "; ", leakCheck(t, b.Binaries[build.Host()], dir))
		})
	}
}
