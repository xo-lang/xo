// Package arm64 holds the tests of the arm64 backend (compiler/arm64)
// through the xo binary (internal/xotool): since decision 0215 the Go
// emitter is gone and this package has no Go code of its own.
package arm64

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

var update = flag.Bool("update", false, "rewrite the golden assembly files")

func TestMain(m *testing.M) {
	flag.Parse()
	// identical builds reuse one binary, within and across runs
	xotool.EnableTestBinCache()
	dir, err := testtmp.Dir("xo-arm64-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	// each xotool.Build in a directory of its own, removed after it
	// (gotchas/property-tests-fill-build-dir.md)
	os.Setenv("XO_TEST_BUILD_SCRATCH", "1")
	code := m.Run()
	xotool.CleanupTemp() // the builds without an output path (xotool.TempDir)
	os.RemoveAll(dir)
	os.Exit(code)
}

// native reports whether binaries of the arm64 backend run here.
func native() bool {
	return runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux")
}

// asm is the arm64 assembly xo emits for dir and goos (`xo asm`).
func asm(t *testing.T, dir, goos string) string {
	t.Helper()
	out, err := xotool.Asm(dir, xotool.BackendArm64, goos+"/arm64")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var benchDir = filepath.Join("..", "..", "bench", "perf", "llvm", "xo")

// TestGoldenAsm compares the assembly of fib's and sieve's own functions
// (ownFuncs: not the std runtime linked with them), for Darwin and Linux,
// with testdata/*.s (-update rewrites them). On a host with clang the
// whole Linux text is also assembled as ELF.
func TestGoldenAsm(t *testing.T) {
	for _, name := range []string{"fib", "sieve"} {
		for _, goos := range []string{"darwin", "linux"} {
			full := asm(t, filepath.Join(benchDir, name), goos)
			got := ownFuncs(full)
			path := filepath.Join("testdata", name+"."+goos+".s")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				continue
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file (run with -update): %v", err)
			}
			if string(want) != got {
				t.Errorf("%s: assembly differs from the golden file (run with -update and review the diff)", path)
			}
			if goos == "linux" {
				whole := filepath.Join(t.TempDir(), name+".s")
				if err := os.WriteFile(whole, []byte(full), 0o644); err != nil {
					t.Fatal(err)
				}
				assembleELF(t, whole)
			}
		}
	}
}

// ownFuncs is the first line of an assembly file (what made it) and the
// functions of the program's main module: the `// func NAME` blocks
// whose NAME has no module prefix, each up to the blank line after it.
// The runtime's functions (std/native modules, `// func mod.name`) are
// left out, so a runtime change does not rewrite the goldens; the
// program output goldens and the fixed point cover the runtime.
func ownFuncs(text string) string {
	lines := strings.SplitAfter(text, "\n")
	var b strings.Builder
	if len(lines) > 0 {
		b.WriteString(lines[0])
	}
	keep := false
	for _, l := range lines[1:] {
		if name, ok := strings.CutPrefix(l, "// func "); ok {
			keep = !strings.Contains(name, ".")
		}
		if keep {
			b.WriteString(l)
			if l == "\n" {
				keep = false
			}
		}
	}
	return b.String()
}

// assembleELF assembles a Linux assembly file with clang's integrated
// assembler, when clang is installed.
func assembleELF(t *testing.T, path string) {
	t.Helper()
	clang, err := exec.LookPath("clang")
	if err != nil {
		return
	}
	obj := filepath.Join(t.TempDir(), "prog.o")
	out, err := exec.Command(clang, "-target", "aarch64-unknown-linux-gnu", "-c", "-o", obj, path).CombinedOutput()
	if err != nil {
		t.Errorf("assembling %s as ELF: %v\n%s", path, err, out)
	}
}

type runResult struct {
	exit           int
	stdout, stderr string
}

func (r runResult) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.exit, r.stdout, r.stderr)
}

var timeRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)

// stableStderr drops the lines of stderr that depend on the backend, as
// internal/llvm's golden test does, and log times.
func stableStderr(s string) string {
	s = timeRE.ReplaceAllString(s, "<time>")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "crash bundle:") || strings.HasPrefix(l, "xo leakcheck:") {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// progArgs are the command line arguments of the golden program in dir
// (the fields of its args.txt) and progEnv its extra environment (the
// NAME=VALUE lines of its env.txt), as internal/llvm's golden test passes
// them.
func progArgs(dir string) []string { return fileFields(filepath.Join(dir, "args.txt")) }
func progEnv(dir string) []string  { return fileFields(filepath.Join(dir, "env.txt")) }

// progStdin is the standard input of the golden program in dir: its
// stdin.txt, or nothing.
func progStdin(dir string) io.Reader {
	data, err := os.ReadFile(filepath.Join(dir, "stdin.txt"))
	if err != nil {
		return nil
	}
	return bytes.NewReader(data)
}

func fileFields(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Fields(string(data))
}

var leakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// buildRun builds dir with the arm64 backend and runs it with the leak
// check on.
func buildRun(t *testing.T, dir string) (runResult, error) {
	t.Helper()
	return buildRunOpts(t, dir, false)
}

func buildRunOpts(t *testing.T, dir string, probes bool) (runResult, error) {
	t.Helper()
	return buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64})
}

// buildRunWith builds dir with opts and runs it with XO_LEAKCHECK=1.
func buildRunWith(t *testing.T, dir string, opts xotool.Options) (runResult, error) {
	t.Helper()
	res, err := xotool.Build([]string{dir}, opts)
	if err != nil {
		return runResult{}, err
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()], progArgs(dir)...))
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), progEnv(dir)...), "XO_LEAKCHECK=1")
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
	if code == 0 {
		m := leakRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Errorf("no leak check line in stderr:\n%s", stderr.String())
		} else if m[1] != "0" {
			t.Errorf("%s allocations still live at exit (of %s)", m[1], m[2])
		}
	}
	return runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(stderr.String())}, nil
}

// goldenDirs returns every program of internal/llvm/testdata/golden.
func goldenDirs(t *testing.T) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "llvm", "testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no golden programs: %v", err)
	}
	return dirs
}

// TestGoldenRun builds every golden program of internal/llvm (read only)
// with the arm64 backend and requires their want.txt: the same exit
// status, stdout, and fault messages as the Go and LLVM backends, and no
// allocation live at exit.
func TestGoldenRun(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	for _, dir := range goldenDirs(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			got, err := buildRun(t, dir)
			if err != nil {
				t.Fatalf("build: %v", err)
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

// TestRun builds the programs of testdata/run with the arm64 backend
// and requires the Go backend's exit status, stdout, and fault messages
// (recorded in testdata/runwant by the Go xo of the go-compiler-final
// tag, decision 0215) and no allocation live at exit.
func TestRun(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	dirs, err := filepath.Glob(filepath.Join("testdata", "run", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no programs: %v", err)
	}
	for _, dir := range dirs {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			got, err := buildRun(t, dir)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			want := goRun(t, dir)
			if want != got.String() {
				t.Errorf("arm64 differs from the Go backend\n--- go\n%s\n--- arm64\n%s", want, got)
			}
		})
	}
}

// goRun is the recorded run of testdata/run/<dir> with the Go backend
// (testdata/runwant/<name>.txt).
func goRun(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "runwant", filepath.Base(dir)+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// benchWant is the output of the benchmark programs (the same with the
// Go, LLVM, and arm64 backends).
var benchWant = map[string]string{
	"fib":     "fib 102334155\n",
	"sieve":   "sieve 3322895\n",
	"nbody":   "nbody -169075163 -169083133\n",
	"sort":    "sort 860218\n",
	"interp":  "interp 999799003\n",
	"collatz": "collatz 428343355 559\n",
	"strings": "strings 12958000\n",
}

// TestBench builds and runs every program of bench/perf/llvm/xo with the
// arm64 backend.
func TestBench(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("runs the benchmark programs")
	}
	for name, want := range benchWant {
		name, want := name, want
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := buildRun(t, filepath.Join(benchDir, name))
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if got.exit != 0 || got.stdout != want {
				t.Errorf("got %s\nwant stdout %q", got, want)
			}
		})
	}
}

// TestLinuxEmit emits fib for linux/arm64 and assembles it as ELF.
func TestLinuxEmit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fib.s")
	if err := os.WriteFile(path, []byte(asm(t, filepath.Join(benchDir, "fib"), "linux")), 0o644); err != nil {
		t.Fatal(err)
	}
	assembleELF(t, path)
}

// The tests of the Go emitter's internals (TestUnsupported on a module
// built by hand, TestInlineVerifies on internal/ssa modules, the layout
// and share probe options of the Go build package) went with decision
// 0215; compiler/arm64 has its own Xo tests (TestSelfhostArm64 compares
// its assembly with the recording).

// TestByRefFieldHome: an inlined `var` helper's field argument (take's
// c.s, golden byref_field_home) is passed at its address in the frame home
// of the local it is a field of, not copied in and out of a home of the
// field's own type at every call (compiler/arm64 byref.xo slot_mems,
// 2026-10-21: frame homes are by type, and another call's field argument
// made one for the field's type). Measured in main's loop (the region from
// a label to the latest branch back to it): at most 16 of the 32 byte
// block copies (`ldp q30, q31`; 24 before) and 225 loads and stores (234
// before); the loop ran 36% faster (20M iterations, 0.48 s to 0.30 s).
func TestByRefFieldHome(t *testing.T) {
	const maxCopies, maxOps = 16, 225
	got := ownFuncs(asm(t, filepath.Join("..", "llvm", "testdata", "golden", "byref_field_home"), "darwin"))
	main := got
	if i := strings.Index(got, "// func main"); i >= 0 {
		main = got[i:]
		if j := strings.Index(main[1:], "// func "); j >= 0 {
			main = main[:j+1]
		}
	}
	lines := strings.Split(main, "\n")
	at := map[string]int{}
	for k, l := range lines {
		if strings.HasSuffix(l, ":") && !strings.HasPrefix(l, "\t") {
			at[strings.TrimSuffix(l, ":")] = k
		}
	}
	lo, hi := -1, -1
	for k, l := range lines {
		f := strings.Fields(l)
		if len(f) < 2 || !(f[0] == "b" || strings.HasPrefix(f[0], "b.") || f[0] == "cbz" || f[0] == "cbnz" || f[0] == "tbz" || f[0] == "tbnz") {
			continue
		}
		if p, ok := at[f[len(f)-1]]; ok && p < k && (lo < 0 || k-p > hi-lo) {
			lo, hi = p, k
		}
	}
	if lo < 0 {
		t.Fatalf("no loop in main:\n%s", main)
	}
	copies, ops := 0, 0
	for _, l := range lines[lo:hi] {
		f := strings.Fields(l)
		if strings.Contains(l, "q30, q31") {
			copies++
		}
		if len(f) > 0 && (f[0] == "ldr" || f[0] == "str" || f[0] == "ldp" || f[0] == "stp") {
			ops++
		}
	}
	if copies > maxCopies || ops > maxOps {
		t.Errorf("main's loop has %d block copies (want at most %d) and %d loads and stores (want at most %d)", copies, maxCopies, ops, maxOps)
	}
}

// TestGoldenRunPoison runs goldens that keep values in frame slots,
// frame homes, and memory areas, built with XO_POISON_FRAMES=1 (each
// frame's words filled with 0xdeadbeefdeadbeef in the prologue): a correct
// program never reads a word it did not write, so the output is the same,
// and a read before any write shows as the poison (2026-10-18: a frame
// home that one loop entry left unwritten read old return addresses).
func TestGoldenRunPoison(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	names := []string{"byref_home_loop", "byref_field_home", "byref_held", "byref_struct", "byref_enum", "var_alias", "value_semantics", "structs", "closures", "maps", "conc_scopes", "fault_unwind", "json_text", "tls_nistec", "http_server", "arg_slots"}
	for _, name := range names {
		dir := filepath.Join("..", "llvm", "testdata", "golden", name)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_POISON_FRAMES=1", "XO_BACKEND_CACHE=off"}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got.String() {
				t.Errorf("output differs from want.txt with XO_POISON_FRAMES=1\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}

// TestGoldenRunSpill builds golden programs whose code has long straight
// blocks, multi-operand templates (the adc chains of cpu_prims and P-256,
// add_with_flag's adds and cset), and faults with the register allocator
// limited to a few registers (XO_ARM64_REGS), so most operands spill:
// every template must still get a distinct register for each spilled
// operand (compiler/arm64 emit.xo scratch_reg; 2026-10-16, before which
// the fourth spilled use, or a second spilled result, shared a scratch
// register with no error).
func TestGoldenRunSpill(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	names := []string{"cpu_prims", "tls_nistec", "crypto_prims", "overflow_add", "neg_overflow", "fault_unwind", "conc_scopes", "json_text", "maps", "closures", "arg_slots", "chain_pads"}
	for _, regs := range []string{"2", "4"} {
		for _, name := range names {
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			if _, err := os.Stat(dir); err != nil {
				continue
			}
			regs, dir := regs, dir
			t.Run(regs+"/"+name, func(t *testing.T) {
				t.Parallel()
				got, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_ARM64_REGS=" + regs, "XO_BACKEND_CACHE=off"}})
				if err != nil {
					t.Fatalf("build: %v", err)
				}
				want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
				if err != nil {
					t.Fatal(err)
				}
				if string(want) != got.String() {
					t.Errorf("output differs from want.txt with XO_ARM64_REGS=%s\n--- want\n%s\n--- got\n%s", regs, want, got)
				}
			})
		}
	}
}
