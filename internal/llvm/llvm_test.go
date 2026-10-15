package llvm

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
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

var updateGolden = flag.Bool("golden-update", false, "rewrite want.txt of the golden programs")

// testDir is the temporary directory TestMain makes and removes.
var testDir string

func TestMain(m *testing.M) {
	flag.Parse()
	// identical builds reuse one binary, within and across runs
	xotool.EnableTestBinCache()
	// identical C compiles (the sanitized drivers) reuse one binary too
	xotool.EnableTestCCache()
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
	xotool.CleanupTemp() // the builds without an output path (xotool.TempDir)
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

func buildAndRun(t *testing.T, dir string, opts xotool.Options) runResult {
	t.Helper()
	r, _ := buildRun(t, dir, opts)
	return r
}

// buildRun builds and runs the program in dir and also returns the build
// result.
func buildRun(t *testing.T, dir string, opts xotool.Options) (runResult, *xotool.Result) {
	t.Helper()
	if why := xotool.Arm64Skip(opts.Backend); why != "" {
		// every arm64 subtest of the diff tests (TestCABIDiff,
		// TestTZDiff, ...) comes through here
		t.Skip(why)
	}
	res, err := xotool.Build([]string{dir}, opts)
	if err != nil {
		t.Fatalf("build (%s): %v", opts.Backend, err)
	}
	if res.Failed {
		if strings.Contains(res.Output, "XO1201") {
			// a construct xo's native emitters do not lower yet: before
			// decision 0215 xo1 handed such a program back to the Go
			// driver (XO1_FALLBACK), which built it; now it is a gap of
			// xo (decision 0215, "Gaps the deletion exposed")
			t.Skipf("xo gap (decision 0215): the %s backend does not build it yet (XO1201):\n%s", opts.Backend, res.Output)
		}
		t.Fatalf("build (%s) failed:\n%s", opts.Backend, res.Output)
	}
	cmd := exec.Command(res.Binaries[xotool.Host()], progArgs(dir)...)
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

// wantFile is a golden program's expected run: want_<goos>.txt when its
// output depends on the target (TestGoldenWant), else want.txt.
func wantFile(dir string) string {
	p := filepath.Join(dir, "want_"+runtime.GOOS+".txt")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return filepath.Join(dir, "want.txt")
}

// exe is the path of a binary the tests compile themselves: with .exe on
// Windows, where exec finds a program by its extension (decision 0197).
func exe(p string) string {
	if runtime.GOOS == "windows" {
		return p + ".exe"
	}
	return p
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
	if _, err := xotool.Clang(); err != nil {
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
			goRes := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo})
			llRes, b := buildRun(t, dir, xotool.Options{Backend: xotool.BackendLLVM})
			if goRes.String() != llRes.String() {
				t.Errorf("LLVM backend differs from the Go backend\n--- go\n%s\n--- llvm\n%s", goRes, llRes)
			}
			if llRes.exit == 0 {
				leakCheck(t, b.Binaries[xotool.Host()], dir)
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

// TestGoldenSplit builds every golden program with its IR split into
// three modules compiled separately (decision 0168), alternating ThinLTO
// and no LTO (shortGoldens both), and requires want.txt and no allocation live at exit.
func TestGoldenSplit(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	type run struct{ dir, lto string }
	var runs []run
	for i, dir := range dirs {
		if shortGoldens[filepath.Base(dir)] {
			runs = append(runs, run{dir, "none"}, run{dir, "thin"})
		} else {
			runs = append(runs, run{dir, []string{"none", "thin"}[i%2]})
		}
	}
	for _, r := range runs {
		dir, lto := r.dir, r.lto
		t.Run(filepath.Base(dir)+"/"+lto, func(t *testing.T) {
			if testing.Short() && !shortGoldens[filepath.Base(dir)] {
				t.Skip("builds every golden program: -short runs shortGoldens")
			}
			t.Parallel()
			res, b := buildRun(t, dir, xotool.Options{Backend: xotool.BackendLLVM, Env: []string{"XO_LLVM_SPLIT=3", "XO_LLVM_JOBS=3", "XO_LLVM_LTO=" + lto}})
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != res.String() {
				t.Errorf("split build differs from want.txt\n--- want\n%s\n--- got\n%s", want, res)
			}
			if res.exit == 0 {
				leakCheck(t, b.Binaries[xotool.Host()], dir)
			}
		})
	}
}

// TestUnsupported and TestBuildErrors checked the errors of the Go LLVM
// emitter and of the Go build options; they went with decision 0215
// (xo's own messages are covered by cmd/xo's tests).

// TestLeakCheck builds each benchmark program (bench/perf/llvm/xo) with
// the LLVM backend and requires the expected output and that
// every allocation is freed by exit (the runtime's allocation counter,
// XO_LEAKCHECK=1).
func TestLeakCheck(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
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
			r, b := buildRun(t, dir, xotool.Options{Backend: xotool.BackendLLVM})
			if r.exit != 0 || !strings.HasPrefix(r.stdout, filepath.Base(dir)+" ") {
				t.Fatalf("run: %s", r)
			}
			t.Log(strings.TrimSpace(r.stdout), "; ", leakCheck(t, b.Binaries[xotool.Host()], dir))
		})
	}
}

// leakExitProg ends with os.proc.exit, which skips the leak report.
const leakExitProg = `fn main(os: Os) {
  os.stdio.println("bye")
  os.proc.exit(3)
}
`

// TestLeakCheckExit: XO_LEAKCHECK=1 on a program that ends with
// os.proc.exit prints that the report was skipped (it runs only when
// main returns), and nothing without XO_LEAKCHECK.
func TestLeakCheckExit(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(leakExitProg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		t.Run(be, func(t *testing.T) {
			r, b := buildRun(t, dir, xotool.Options{Backend: be})
			if r.exit != 3 || r.stdout != "bye\n" || strings.Contains(r.stderr, "xo leakcheck") {
				t.Fatalf("run without XO_LEAKCHECK: %s", r)
			}
			cmd := exec.Command(b.Binaries[xotool.Host()])
			cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			var ee *exec.ExitError
			if !errors.As(err, &ee) || ee.ExitCode() != 3 {
				t.Fatalf("exit: %v", err)
			}
			if stdout.String() != "bye\n" || stderr.String() != "xo leakcheck: skipped (os.proc.exit)\n" {
				t.Fatalf("stdout %q, stderr %q", stdout.String(), stderr.String())
			}
		})
	}
}

// TestNoBareLibcExportsInLibcBuild: the C library functions of
// cabi_libc.xo are exported under their C names only in an XO_NOLIBC
// build (decision 0230); in a build that links the system's C library
// a bare malloc of Xo's would silently replace the system's. A program
// built with each native backend defines none of them.
func TestNoBareLibcExportsInLibcBuild(t *testing.T) {
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	if testing.Short() {
		t.Skip("builds a program")
	}
	nm, err := exec.LookPath("nm")
	if err != nil {
		t.Skip("no nm")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(leakExitProg), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
		t.Run(be, func(t *testing.T) {
			_, b := buildRun(t, dir, xotool.Options{Backend: be})
			out, err := exec.Command(nm, "-g", b.Binaries[xotool.Host()]).Output()
			if err != nil {
				t.Fatalf("nm: %v", err)
			}
			for _, line := range strings.Split(string(out), "\n") {
				f := strings.Fields(line)
				if len(f) != 3 || f[1] != "T" {
					continue
				}
				switch strings.TrimPrefix(f[2], "_") {
				case "malloc", "free", "calloc", "realloc":
					t.Errorf("the program defines %s (cabi_libc.xo exported in a libc build)", f[2])
				}
			}
		})
	}
}
