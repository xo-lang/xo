package arm64_test

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
	"sync/atomic"
	"testing"

	"github.com/xo-lang/xo/internal/arm64"
	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
	"github.com/xo-lang/xo/internal/testtmp"
)

var update = flag.Bool("update", false, "rewrite the golden assembly files")

func TestMain(m *testing.M) {
	flag.Parse()
	// identical builds reuse one binary, within and across runs
	build.EnableTestBinCache()
	dir, err := testtmp.Dir("xo-arm64-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// native reports whether binaries of the arm64 backend run here.
func native() bool {
	return runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux")
}

func asm(t *testing.T, dir, goos string) string {
	t.Helper()
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
	out, err := build.Arm64Asm(res, abs, goos)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return string(out)
}

var benchDir = filepath.Join("..", "..", "bench", "perf", "llvm", "xo")

// TestGoldenAsm compares the assembly of fib and sieve, for Darwin and
// Linux, with testdata/*.s (-update rewrites them). On a host with clang
// the Linux text is also assembled as ELF.
func TestGoldenAsm(t *testing.T) {
	for _, name := range []string{"fib", "sieve"} {
		for _, goos := range []string{"darwin", "linux"} {
			got := asm(t, filepath.Join(benchDir, name), goos)
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
				assembleELF(t, path)
			}
		}
	}
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

// buildRunOpts is buildRun, with share probes when probes is set
// (build.Options.ShareProbes).
func buildRunOpts(t *testing.T, dir string, probes bool) (runResult, error) {
	t.Helper()
	return buildRunWith(t, dir, build.Options{Mode: gen.Build, Backend: build.BackendArm64, ShareProbes: probes})
}

// buildRunWith builds dir with opts and runs it with XO_LEAKCHECK=1.
func buildRunWith(t *testing.T, dir string, opts build.Options) (runResult, error) {
	t.Helper()
	res, err := build.Build([]string{dir}, opts)
	if err != nil {
		return runResult{}, err
	}
	if res.Failed {
		t.Fatalf("check failed")
	}
	cmd := exec.Command(res.Binaries[build.Host()], progArgs(dir)...)
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

// TestGoldenRunShared builds every golden program of internal/llvm with
// share probes (every heap value marked shared between threads as it is
// defined, decision 0058) and requires want.txt and no allocation live
// at exit; the Linux assembly (exclusive load and store loops instead of
// LSE atomics) must assemble as ELF.
func TestGoldenRunShared(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	lin := build.Target{OS: "linux", Arch: "arm64"}
	for _, dir := range goldenDirs(t) {
		dir := dir
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			got, err := buildRunOpts(t, dir, true)
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
			if build.Host() != lin {
				res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: build.BackendArm64, ShareProbes: true, Targets: []build.Target{lin}})
				if err != nil {
					t.Fatal(err)
				}
				assembleELF(t, res.Asm[lin])
			}
		})
	}
}

// TestRun builds the programs of testdata/run with the Go backend and
// the arm64 backend and requires the same exit status, stdout, and
// fault messages (and no allocation live at exit on arm64).
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
			if want.String() != got.String() {
				t.Errorf("arm64 differs from the Go backend\n--- go\n%s\n--- arm64\n%s", want, got)
			}
		})
	}
}

// goRun builds dir with the Go backend and runs it.
func goRun(t *testing.T, dir string) runResult {
	t.Helper()
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: build.BackendGo})
	if err != nil || res.Failed {
		t.Fatalf("go build: %v", err)
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
	return runResult{exit: code, stdout: stdout.String(), stderr: stableStderr(stderr.String())}
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

// TestUnsupported checks that an op outside what the backend lowers is
// rejected with an error, not miscompiled. Every op the builder and the
// RC pass emit is lowered, so the module is built by hand: OpDrop (drop
// specialization) is declared in internal/ssa but never emitted yet.
func TestUnsupported(t *testing.T) {
	m := ssa.NewModule()
	str := m.Builtin(ssa.KStr)
	f := m.NewFunc("main", []*ssa.Type{str}, []*ssa.Type{m.Builtin(ssa.KUnit)})
	b := f.Blocks[0]
	b.Add(ssa.OpDrop, m.Builtin(ssa.KUnit), nil, f.Params()[0])
	u := b.Add(ssa.OpTuple, m.Builtin(ssa.KUnit), nil)
	b.Add(ssa.OpReturn, nil, nil, u)
	m.RC = true
	_, err := arm64.Generate(m, arm64.Options{OS: arm64.Darwin, NoInline: true})
	var ae *arm64.Error
	if !errors.As(err, &ae) || !strings.Contains(err.Error(), "drop") {
		t.Fatalf("got %v, want an unsupported op error naming drop", err)
	}
}

// TestBuildErrors covers the options the arm64 backend rejects.
func TestBuildErrors(t *testing.T) {
	dir := filepath.Join(benchDir, "fib")
	cases := []struct {
		opts build.Options
		want string
	}{
		{build.Options{Mode: gen.Build, Backend: build.BackendArm64, Targets: []build.Target{{OS: "linux", Arch: "amd64"}}}, "darwin/arm64 and linux/arm64"},
	}
	for _, c := range cases {
		_, err := build.Build([]string{dir}, c.opts)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want an error containing %q", c.opts, err, c.want)
		}
	}
}

// TestLinuxEmit builds for linux/arm64: on another host the assembly is
// written and not linked.
func TestLinuxEmit(t *testing.T) {
	lin := build.Target{OS: "linux", Arch: "arm64"}
	res, err := build.Build([]string{filepath.Join(benchDir, "fib")}, build.Options{Mode: gen.Build, Backend: build.BackendArm64, Targets: []build.Target{lin}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Asm[lin] == "" {
		t.Fatal("no assembly for linux/arm64")
	}
	if build.Host() != lin && res.Binaries[lin] != "" {
		t.Errorf("linked a binary for linux/arm64 on %s", build.Host())
	}
	assembleELF(t, res.Asm[lin])
}

// TestInlineVerifies requires every program to verify (reference
// counting balance included) after the backend inlines small functions:
// build.Arm64Asm would otherwise fall back to no inlining silently.
func TestInlineVerifies(t *testing.T) {
	var dirs []string
	for name := range benchWant {
		dirs = append(dirs, filepath.Join(benchDir, name))
	}
	dirs = append(dirs, goldenDirs(t)...)
	runs, _ := filepath.Glob(filepath.Join("testdata", "run", "*"))
	dirs = append(dirs, runs...)
	var inlined atomic.Int64
	// the programs in parallel; the group returns when all are done
	t.Run("all", func(t *testing.T) {
		for _, dir := range dirs {
			dir := dir
			t.Run(filepath.Base(dir), func(t *testing.T) {
				t.Parallel()
				res, ok, err := build.CheckNative([]string{dir}, false)
				if err != nil || !ok {
					t.Fatalf("%s: check failed: %v", dir, err)
				}
				m, err := ssa.Build(res)
				if err != nil {
					t.Fatalf("%s: %v", dir, err)
				}
				m.InsertRC()
				before := len(m.Funcs[0].Blocks)
				for _, f := range m.Funcs {
					before += len(f.Blocks)
				}
				if _, err := arm64.Generate(m, arm64.Options{OS: arm64.Darwin}); err != nil {
					t.Errorf("%s: %v", dir, err)
				}
				after := len(m.Funcs[0].Blocks)
				for _, f := range m.Funcs {
					after += len(f.Blocks)
				}
				if after != before {
					inlined.Add(1)
				}
			})
		}
	})
	if inlined.Load() == 0 {
		t.Error("nothing was inlined")
	}
}

// TestLayoutRun builds the bench programs and the loop heavy run programs
// with code placement settings ($XO_ARM64_LAYOUT, decision 0075): loop
// tops aligned with a jump over the padding or nops run through, outer
// loops, offsets, functions moved by nops. The output must not change.
func TestLayoutRun(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	runs := []string{"lsr", "lsr_fault_neg", "latch", "switch", "ranges", "select", "fault_bounds"}
	// the Go backend's output of the run programs, once each
	goWant := map[string]string{}
	for _, r := range runs {
		goWant[r] = goRun(t, filepath.Join("testdata", "run", r)).String()
	}
	for _, lay := range []string{"func=6,loop=6,pad=3", "func=4,loop=5,fall=1,off=3,outer=1,pad=1"} {
		opts := build.Options{Mode: gen.Build, Backend: build.BackendArm64, ARM64Layout: lay}
		for name, want := range benchWant {
			name, want := name, want
			t.Run(lay+"/"+name, func(t *testing.T) {
				t.Parallel()
				got, err := buildRunWith(t, filepath.Join(benchDir, name), opts)
				if err != nil {
					t.Fatalf("%s: build: %v", name, err)
				}
				if got.exit != 0 || got.stdout != want {
					t.Errorf("%s: got %s\nwant stdout %q", name, got, want)
				}
			})
		}
		for _, r := range runs {
			r := r
			t.Run(lay+"/"+r, func(t *testing.T) {
				t.Parallel()
				got, err := buildRunWith(t, filepath.Join("testdata", "run", r), opts)
				if err != nil {
					t.Fatalf("%s: build: %v", r, err)
				}
				if got.String() != goWant[r] {
					t.Errorf("%s: arm64 differs from the Go backend\n--- go\n%s\n--- arm64\n%s", r, goWant[r], got)
				}
			})
		}
	}
}
