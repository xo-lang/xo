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
)

var updateGolden = flag.Bool("golden-update", false, "rewrite want.txt of the golden programs")

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "xo-llvm-test-")
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
	cmd := exec.Command(res.Binaries[build.Host()])
	cmd.Dir = dir
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

// ssaGolden lists the golden programs that must take the SSA path (with
// reference counting): a silent fallback to the AST path fails the test.
var ssaGolden = map[string]bool{
	"abs_fault": true, "arith": true, "div_zero": true, "float_int_fault": true, "index_oob": true,
	"loops": true, "neg_overflow": true, "overflow_add": true, "overflow_mul": true, "rc_values": true,
	"shl_overflow": true, "sort_structs": true, "strings": true, "user_fault": true, "var_alias": true,
}

// leakCheck runs bin with XO_LEAKCHECK=1 and requires every allocation
// freed at exit.
func leakCheck(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
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

var leakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// TestGolden builds every program under testdata/golden with the Go
// backend and with the LLVM backend (with and without the range
// analysis) and requires the same exit status, stdout, and fault
// messages, which must also match want.txt.
func TestGolden(t *testing.T) {
	if _, err := build.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds every golden program three times")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			name := filepath.Base(dir)
			goRes := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendGo})
			llRes, b := buildRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM})
			if goRes.String() != llRes.String() {
				t.Errorf("LLVM backend (%s path) differs from the Go backend\n--- go\n%s\n--- llvm\n%s", b.LLVMPath, goRes, llRes)
			}
			if ssaGolden[name] && b.LLVMPath != "ssa" {
				t.Errorf("took the %s path, want ssa: %s", b.LLVMPath, b.LLVMFallback)
			}
			if b.LLVMPath == "ssa" && llRes.exit == 0 {
				leakCheck(t, b.Binaries[build.Host()], dir)
			}
			// the prototype AST path, with and without the range analysis
			astRes := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM, LLVMAST: true})
			if astRes.String() != llRes.String() {
				t.Errorf("LLVM AST path differs\n--- default\n%s\n--- ast\n%s", llRes, astRes)
			}
			noRanges := buildAndRun(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendLLVM, LLVMAST: true, NoRanges: true})
			if noRanges.String() != llRes.String() {
				t.Errorf("LLVM AST path without range analysis differs\n--- ranges\n%s\n--- no ranges\n%s", llRes, noRanges)
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
			res, ok, err := build.Check([]string{dir}, false)
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
		{build.Options{Mode: gen.Test, Backend: build.BackendLLVM}, "programs only"},
		{build.Options{Mode: gen.Build, Backend: build.BackendLLVM, Targets: []build.Target{{OS: "plan9", Arch: "amd64"}}}, "host"},
		{build.Options{Mode: gen.Build, Backend: build.BackendLLVM, Arenas: true}, "--arenas"},
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
// the LLVM backend, requires the SSA path, the expected output, and that
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
			if b.LLVMPath != "ssa" {
				t.Fatalf("took the %s path, want ssa: %s", b.LLVMPath, b.LLVMFallback)
			}
			if r.exit != 0 || !strings.HasPrefix(r.stdout, filepath.Base(dir)+" ") {
				t.Fatalf("run: %s", r)
			}
			t.Log(strings.TrimSpace(r.stdout), "; ", leakCheck(t, b.Binaries[build.Host()], dir))
		})
	}
}
