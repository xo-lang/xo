package arm64_test

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
)

var update = flag.Bool("update", false, "rewrite the golden assembly files")

func TestMain(m *testing.M) {
	flag.Parse()
	dir, err := os.MkdirTemp("", "xo-arm64-test-")
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

// stableStderr drops the lines of stderr that depend on the backend, as
// internal/llvm's golden test does.
func stableStderr(s string) string {
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

var leakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// buildRun builds dir with the arm64 backend and runs it with the leak
// check on.
func buildRun(t *testing.T, dir string) (runResult, error) {
	t.Helper()
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: build.BackendArm64})
	if err != nil {
		return runResult{}, err
	}
	if res.Failed {
		t.Fatalf("check failed")
	}
	cmd := exec.Command(res.Binaries[build.Host()])
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_LEAKCHECK=1")
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

// nativeGolden lists the programs of internal/llvm/testdata/golden that
// must build with the arm64 backend (the ones the SSA path supports).
var nativeGolden = []string{
	"abs_fault", "arith", "div_zero", "float_int_fault", "index_oob", "loops", "neg_overflow", "overflow_add",
	"overflow_mul", "rc_aliasing", "rc_values", "shl_overflow", "sort_structs", "strings", "user_fault", "var_alias",
}

// TestGoldenRun builds the golden programs of internal/llvm (read only)
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
	for _, name := range nativeGolden {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
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

// TestUnsupported checks that a program outside the SSA subset is
// rejected with an error, not miscompiled.
func TestUnsupported(t *testing.T) {
	dir := filepath.Join("..", "llvm", "testdata", "golden", "closures")
	_, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Backend: build.BackendArm64})
	if err == nil {
		t.Fatal("build succeeded; want an unsupported construct error")
	}
}

// TestBuildErrors covers the options the arm64 backend rejects.
func TestBuildErrors(t *testing.T) {
	dir := filepath.Join(benchDir, "fib")
	cases := []struct {
		opts build.Options
		want string
	}{
		{build.Options{Mode: gen.Test, Backend: build.BackendArm64}, "programs only"},
		{build.Options{Mode: gen.Build, Backend: build.BackendArm64, Targets: []build.Target{{OS: "linux", Arch: "amd64"}}}, "darwin/arm64 and linux/arm64"},
		{build.Options{Mode: gen.Build, Backend: build.BackendArm64, Arenas: true}, "--arenas"},
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
