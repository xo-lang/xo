package xotool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// The backends of `xo build --backend=` and the modes of Build.
const (
	BackendGo    = "go"
	BackendArm64 = "arm64"
	BackendLLVM  = "llvm"
	BackendWasm  = "wasm"

	ModeBuild = "build"
	ModeTest  = "test"
)

// Options are a build through the xo binary (Path), for the Go tests
// that build Xo programs (they called the Go compiler's build package in
// process before decision 0215).
type Options struct {
	// Mode is ModeBuild (`xo build`, the default) or ModeTest (`xo
	// test`, which builds the test binary and runs it; the binary stays
	// at Out).
	Mode string
	// Backend: "" is the Go backend, named explicitly (xo's own default
	// is native since decision 0248; these tests were written for Go),
	// or the native one with NoGC
	Backend string
	NoGC    bool
	// Out is the binary's path; "" is a fresh file in a temporary
	// directory (the caller's to remove: Result.Dir).
	Out string
	// Targets is --targets (os/arch, comma separated, or all).
	Targets string
	// TZData is --tzdata (embedded or system).
	TZData string
	// Release, NoErrorArgs, Race, NoArenas, NoMono, Holes, and Hybrid
	// are --release, --error-args=false, --race, --no-arenas,
	// --no-mono, --holes, and --hybrid.
	Release, NoErrorArgs, Race, NoArenas, NoMono, Holes, Hybrid bool
	// Flags are more flags.
	Flags []string
	// Env are more NAME=VALUE environment entries of the xo process.
	Env []string
	// Dir is the working directory of the xo process ("" is this
	// process's).
	Dir string
}

// Result is what a Build made.
type Result struct {
	// Binaries maps each target xo built ("os/arch" in Go's names:
	// wasip1/wasm, not wasi/wasm32; Target converts) to its binary;
	// a single target build's binary is under Host.
	Binaries map[string]string
	// Failed reports that xo exited with an error and wrote no binary
	// (a check error, a build error); Output says why.
	Failed bool
	// Code is xo's exit status (a test run's, in ModeTest).
	Code int
	// Output is xo's standard output and standard error.
	Output string
	// Dir is the temporary directory holding Out when Options.Out was "".
	Dir string
	// Diagnostics are `xo check --json`'s for the dirs when the build
	// failed (nil when the check passes: a build or link error).
	Diagnostics []Diagnostic
	// Notes are the lines of Output that are notes (`: note: `).
	Notes []string
}

// Diagnostic is one diagnostic of xo's JSON output (spec/core.md 10).
type Diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Span     struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Col     int    `json:"col"`
		EndLine int    `json:"end_line"`
		EndCol  int    `json:"end_col"`
	} `json:"span"`
	Notes []json.RawMessage `json:"notes"`
	Fixes []json.RawMessage `json:"fixes"`
}

// Human is the diagnostic as one line: file:line:col: severity code: message.
func (d Diagnostic) Human() string {
	return fmt.Sprintf("%s:%d:%d: %s %s: %s", d.Span.File, d.Span.Line, d.Span.Col, d.Severity, d.Code, d.Message)
}

// Host is this machine's target, "os/arch".
func Host() string { return runtime.GOOS + "/" + runtime.GOARCH }

// Target is the os/arch name xo prints for target s and Result.Binaries
// is keyed by (build.ParseTargets of the Go compiler, parse_targets of
// compiler/xo): Go's names, so the spec's wasi/wasm32 (core 12.1) is
// wasip1/wasm; other names are returned as they are.
func Target(s string) string {
	os_, arch, ok := strings.Cut(strings.TrimSpace(s), "/")
	if !ok {
		return s
	}
	if os_ == "wasi" {
		os_ = "wasip1"
	}
	if arch == "wasm32" {
		arch = "wasm"
	}
	return os_ + "/" + arch
}

// Build builds dirs with the xo binary (Path): `xo build|test
// [--backend=B] [--no-gc] [flags] -o OUT dirs...`. A missing xo is the
// error; a failing build is Result.Failed.
func Build(dirs []string, o Options) (*Result, error) {
	cli, err := Command()
	if err != nil {
		return nil, err
	}
	mode := o.Mode
	if mode == "" {
		mode = ModeBuild
	}
	res := &Result{Binaries: map[string]string{}}
	out := o.Out
	if out == "" {
		d, err := TempDir("xo-build-")
		if err != nil {
			return nil, err
		}
		res.Dir = d
		name := "prog"
		if len(dirs) > 0 {
			name = filepath.Base(filepath.Clean(dirs[0]))
		}
		out = filepath.Join(d, name)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(out, ".exe") {
		out += ".exe"
	}
	args := append([]string{}, cli[1:]...)
	args = append(args, mode)
	if o.Backend != "" {
		args = append(args, "--backend="+o.Backend)
	} else if !o.NoGC {
		args = append(args, "--backend="+BackendGo)
	}
	for _, f := range []struct {
		on   bool
		flag string
	}{
		{o.NoGC, "--no-gc"}, {o.Targets != "", "--targets=" + o.Targets}, {o.TZData != "", "--tzdata=" + o.TZData},
		{o.Release, "--release"}, {o.NoErrorArgs, "--error-args=false"}, {o.Race, "--race"}, {o.NoArenas, "--no-arenas"},
		{o.NoMono, "--no-mono"}, {o.Holes, "--holes"}, {o.Hybrid, "--hybrid"},
	} {
		if f.on {
			args = append(args, f.flag)
		}
	}
	args = append(args, o.Flags...)
	args = append(args, "-o", out)
	args = append(args, dirs...)
	if mode == ModeTest {
		// `xo test` writes shrunk contract fuzz failures into the module
		// (regressions_test.xo, core 9.2); a test's directory stays as
		// it was (two golden programs were committed with one)
		for _, d := range dirs {
			p := filepath.Join(d, "regressions_test.xo")
			if o.Dir != "" && !filepath.IsAbs(p) {
				p = filepath.Join(o.Dir, p)
			}
			if _, err := os.Stat(p); err != nil {
				defer os.Remove(p)
			}
		}
	}
	cmd := exec.Command(cli[0], args...)
	cmd.Dir = o.Dir
	wd := o.Dir
	if wd == "" {
		wd, _ = os.Getwd()
	}
	env := append(append(os.Environ(), "PWD="+wd), o.Env...)
	if bd := buildScratch(mode, o.Env); bd != "" {
		defer os.RemoveAll(bd)
		env = append(env, "XO_BUILD_DIR="+bd)
	}
	cmd.Env = TestJobsEnv(env)
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err = cmd.Run()
	res.Output = b.String()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		res.Code = ee.ExitCode()
	default:
		return nil, fmt.Errorf("%s: %v", cli[0], err)
	}
	// `xo build` prints `os/arch: path` for each target it built
	for _, l := range strings.Split(res.Output, "\n") {
		if m := targetLine.FindStringSubmatch(l); m != nil {
			if fi, serr := os.Stat(m[2]); serr == nil && fi.Mode().IsRegular() {
				res.Binaries[m[1]] = m[2]
			}
		}
	}
	if fi, serr := os.Stat(out); serr == nil && fi.Mode().IsRegular() && res.Binaries[Host()] == "" {
		res.Binaries[Host()] = out
	}
	if _, ok := res.Binaries[Host()]; !ok || (mode == ModeBuild && res.Code != 0) {
		res.Failed = true
	}
	for _, l := range strings.Split(res.Output, "\n") {
		if strings.Contains(l, ": note: ") {
			res.Notes = append(res.Notes, l)
		}
	}
	if res.Failed {
		cargs := append(append([]string{}, cli[1:]...), "check", "--json")
		if o.NoGC {
			cargs = append(cargs, "--no-gc")
		}
		c := exec.Command(cli[0], append(cargs, dirs...)...)
		c.Dir, c.Env = cmd.Dir, cmd.Env
		out, _ := c.Output()
		var r struct {
			Diagnostics []Diagnostic `json:"diagnostics"`
		}
		if json.Unmarshal(out, &r) == nil {
			res.Diagnostics = r.Diagnostics
		}
	}
	return res, nil
}

// buildScratch is a fresh directory under the test package's
// $XO_BUILD_DIR for one `xo build`, which Build removes after it: xo
// never prunes a build directory base the user chose (compiler/xo
// cache.xo build_dir), and each new program adds a directory of its
// generated code and objects, so a package's runs grew without bound
// (18 GB in 17 minutes of internal/gen's property tests, 11 GB of
// internal/llvm's full suite; gotchas/property-tests-fill-build-dir.md).
// The binary is at Out, outside it. Only in a package whose TestMain
// sets XO_TEST_BUILD_SCRATCH=1 (internal/gen, internal/llvm,
// internal/arm64: cmd/xo's tests read their build directories); ""
// (xo's own build directory) otherwise, when the caller sets
// XO_BUILD_DIR in env, the mode is not ModeBuild, or with
// XO_TEST_KEEP_BUILD=1.
func buildScratch(mode string, env []string) string {
	base := os.Getenv("XO_BUILD_DIR")
	if base == "" || os.Getenv("XO_TEST_BUILD_SCRATCH") != "1" || mode != ModeBuild || os.Getenv("XO_TEST_KEEP_BUILD") == "1" {
		return ""
	}
	for _, e := range env {
		if strings.HasPrefix(e, "XO_BUILD_DIR=") {
			return ""
		}
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return ""
	}
	d, err := os.MkdirTemp(base, "b-")
	if err != nil {
		return ""
	}
	return d
}

// Clang is the C compiler of the LLVM backend, as xo finds it: $XO_CLANG,
// else clang on PATH.
func Clang() (string, error) {
	// the test clang cache's real clang (EnableTestClangCache)
	if c := os.Getenv(clangRealEnv); c != "" {
		return exec.LookPath(c)
	}
	if c := os.Getenv("XO_CLANG"); c != "" {
		return exec.LookPath(c)
	}
	p, err := exec.LookPath("clang")
	if err != nil {
		return "", errors.New("the LLVM backend needs clang: install it or set XO_CLANG")
	}
	return p, nil
}

// CC is the C compiler that assembles and links arm64 builds, as xo
// finds it: $XO_CC, else cc, else clang on PATH.
func CC() (string, error) {
	if c := os.Getenv("XO_CC"); c != "" {
		return exec.LookPath(c)
	}
	for _, n := range []string{"cc", "clang"} {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", errors.New("the arm64 backend needs a C compiler (cc or clang) or XO_CC")
}

// NativeBackend is the native backend of this host: arm64 on a darwin
// or linux arm64 host, else llvm.
func NativeBackend() string {
	if Arm64Host() {
		return BackendArm64
	}
	return BackendLLVM
}

// Arm64Host reports whether this host can build and run the arm64
// backend's programs: darwin/arm64 or linux/arm64. Elsewhere `xo build
// --backend=arm64` hands arm64 assembly to the host's assembler (an x86
// one fails on `stp x29,x30,...`), so a test skips the arm64 backend
// (Arm64Skip; gotchas/arm64-tests-on-x86-hosts.md).
func Arm64Host() bool {
	return runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux")
}

// Arm64Skip is why backend cannot build on this host ("" when it can):
// the arm64 backend off an arm64 host (Arm64Host).
func Arm64Skip(backend string) string {
	if backend == BackendArm64 && !Arm64Host() {
		return "the arm64 backend builds on darwin/arm64 and linux/arm64 hosts, not " + Host()
	}
	return ""
}

// EnableTestBinCache points $XO_BIN_CACHE (xo's cache of linked
// binaries) at <user cache>/xo/testbin unless it is set, so identical
// test builds reuse one binary within and across runs.
func EnableTestBinCache() {
	if _, ok := os.LookupEnv("XO_BIN_CACHE"); ok {
		return
	}
	if c, err := os.UserCacheDir(); err == nil {
		os.Setenv("XO_BIN_CACHE", filepath.Join(c, "xo", "testbin"))
	}
}

var targetLine = regexp.MustCompile(`^([a-z0-9]+/[a-z0-9]+): (\S.*)$`)

// LLVMIR is the LLVM IR xo emits for the program in dir (Asm with the
// LLVM backend, one module): what the runtime tests compile with the C
// runtime and sanitizer flags themselves (they called the Go emitter's
// llvm.Generate before decision 0215).
func LLVMIR(dir string) ([]byte, error) { return Asm(dir, BackendLLVM, "") }

// LLVMIRRace is LLVMIR with --race (decision 0241): every function
// marked sanitize_thread, for a build with clang's -fsanitize=thread.
func LLVMIRRace(dir string) ([]byte, error) {
	return asm(dir, BackendLLVM, "", "--race")
}

// LLVMIRRaceRuntime is LLVMIRRace with the runtime instrumented too
// (XO_TSAN_RUNTIME=1, decision 0273 mode 1): the runtime's own race tests,
// whose binary also defines xo_tsan_runtime (internal/llvm
// tsanBuildRuntime).
func LLVMIRRaceRuntime(dir string) ([]byte, error) {
	return asmEnv(dir, BackendLLVM, "", []string{"XO_TSAN_RUNTIME=1"}, "--race")
}

// Asm is `xo asm --backend=backend [--target=target] dir`: the assembly
// (arm64) or the LLVM IR (llvm, one module) xo emits for the program in
// dir, for the host or target ("" is the host).
func Asm(dir, backend, target string) ([]byte, error) { return asm(dir, backend, target) }

func asm(dir, backend, target string, flags ...string) ([]byte, error) {
	return asmEnv(dir, backend, target, nil, flags...)
}

func asmEnv(dir, backend, target string, env []string, flags ...string) ([]byte, error) {
	cli, err := Command()
	if err != nil {
		return nil, err
	}
	args := append(append(append([]string{}, cli[1:]...), "asm", "--backend="+backend), flags...)
	if target != "" {
		args = append(args, "--target="+target)
	}
	cmd := exec.Command(cli[0], append(args, dir)...)
	cmd.Env = append(append(os.Environ(), "XO_LLVM_SPLIT=1"), env...)
	if os.Getenv("XO_NOLIBC") == "" {
		// the tests link this code with clang and the C library (the
		// sanitizers among them), or compare it with assembly recorded
		// so: since decision 0243 a Linux program has no C library by
		// default, its own _start
		cmd.Env = append(cmd.Env, "XO_NOLIBC=0")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xo %s %s: %v\n%s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return out, nil
}

// Check is `xo check --json [flags] dirs`: the diagnostics (none when
// the program checks).
func Check(dirs []string, flags ...string) ([]Diagnostic, error) {
	cli, err := Command()
	if err != nil {
		return nil, err
	}
	args := append(append([]string{}, cli[1:]...), "check", "--json")
	args = append(append(args, flags...), dirs...)
	cmd := exec.Command(cli[0], args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var r struct {
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		return nil, fmt.Errorf("xo check --json: %v %v\n%s%s", err, jerr, out, stderr.String())
	}
	return r.Diagnostics, nil
}
