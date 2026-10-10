// Package build compiles checked Xo programs: it runs the checker, the
// code generator (internal/gen), writes a Go build directory with the
// runtime (std/rt), and invokes `go build` for each target with
// CGO_ENABLED=0 (decision 0002, core 12.1).
package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/std"
)

// Target is a GOOS/GOARCH pair.
type Target struct {
	OS, Arch string
}

func (t Target) String() string { return t.OS + "/" + t.Arch }

// AllTargets are the targets of `xo build --targets=all` (core 12.1).
// `wasi/wasm32` is Go's wasip1/wasm.
var AllTargets = []Target{
	{"linux", "amd64"}, {"linux", "arm64"}, {"darwin", "amd64"}, {"darwin", "arm64"},
	{"windows", "amd64"}, {"windows", "arm64"}, {"wasip1", "wasm"},
}

// Host is the target of the running toolchain.
func Host() Target { return Target{runtime.GOOS, runtime.GOARCH} }

// ParseTargets parses `all` or a comma separated list of os/arch.
func ParseTargets(s string) ([]Target, error) {
	if s == "" {
		return []Target{Host()}, nil
	}
	if s == "all" {
		return AllTargets, nil
	}
	var out []Target
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		os_, arch, ok := strings.Cut(part, "/")
		if !ok || os_ == "" || arch == "" {
			return nil, fmt.Errorf("bad target %q: want os/arch", part)
		}
		if os_ == "wasi" {
			os_ = "wasip1"
		}
		if arch == "wasm32" {
			arch = "wasm"
		}
		out = append(out, Target{os_, arch})
	}
	return out, nil
}

// Options configure a build.
type Options struct {
	Mode    gen.Mode
	Holes   bool
	Targets []Target
	// Out is the output binary path (one target) or directory (several);
	// empty means the build directory.
	Out string
	// Race builds with the race detector (host only).
	Race bool
	// NoMono turns off the specialization of generic code per
	// instantiation (gen.Options.NoMono, `--no-mono`).
	NoMono bool
	// NoRanges checks every Int operation (gen.Options.NoRanges; tests).
	NoRanges bool
	// NoArenas turns off request scoped arenas (gen.Options.Arenas,
	// `--no-arenas`). Arenas are on by default since 2026-10-08 (decision
	// 0066) on the Go backend; the llvm and arm64 backends have no HTTP
	// server and ignore them.
	NoArenas bool
	// Backend is "go" (the default, P1), "llvm" (the P3 prototype,
	// internal/llvm: IR compiled by clang; host only, programs only), or
	// "arm64" (internal/arm64: assembly linked with cc; darwin/arm64 and
	// linux/arm64).
	Backend string
	// NoGC (`--no-gc`, decision 0083) builds without a garbage collector:
	// on a native backend, NativeBackend() unless Backend names llvm or
	// arm64 (Backend go is an error).
	NoGC bool
	// ShareProbes (LLVM and arm64 backends; tests) marks every heap
	// value shared between threads as it is defined, so every count
	// update takes the atomic path (ssa.InsertShareProbes, decision
	// 0058).
	ShareProbes bool
	// Hybrid (`--hybrid`, decision 0084, Go backend programs) compiles
	// the hot leaf functions of the main module to Go assembler with the
	// arm64 backend (hybrid.go); HybridAll takes every eligible function
	// (tests), and HybridPure builds the same directory with the Go
	// versions (the tag xo_pure: the reference of differential tests).
	Hybrid, HybridAll, HybridPure bool
	// Stderr receives compiler output.
	Stderr *bytes.Buffer
	// TZData is where std/time finds zone data (decision 0094): "" or
	// "embedded" embeds the zones the program uses (tzdata.go), "system"
	// reads the host database at run time (`--tzdata=system`).
	TZData string
	// tags are the program's Go build tags (gen.Program.Tags).
	tags []string
	// Release (`xo build --release`, core 8) is a release build: the
	// contract clauses marked `requires(debug)` and `ensures(debug)` are
	// compiled out. It applies to Build mode only; tests are debug builds.
	Release bool
	// NoErrorArgs (`--error-args=false`, release builds only, decision
	// 0092) records error frames without parameter values.
	NoErrorArgs bool
	// ARM64Layout is the arm64 backend's code placement in the form of
	// $XO_ARM64_LAYOUT (decision 0075), which it overrides when set; the
	// build gets its own directory, so tests build several placements at
	// once.
	ARM64Layout string
}

// release reports whether this is a release build (Release, Build mode).
func (o Options) release() bool { return o.Release && o.Mode == gen.Build }

// releaseVariant is the build directory variant of a release build.
func releaseVariant(o Options) string {
	if o.release() {
		return "release"
	}
	return ""
}

// Result is the outcome of a build.
type Result struct {
	Diagnostics []diag.Diagnostic
	// Failed is set when checking reported errors (no Go was built).
	Failed bool
	// Binaries maps each target to its binary path.
	Binaries map[Target]string
	// Dir is the Go build directory.
	Dir string
	// Name is the program name (the main module's directory name).
	Name string
	// Specs counts the specialized copies of generic functions.
	Specs int
	// Asm maps each target of the arm64 backend to its assembly file
	// (a target other than the host is emitted, not linked).
	Asm map[Target]string
	// Info is the build information compiled into a program (Build mode,
	// Go backend): the build id, the source hashes, the main module
	// directory, and the options, for crash bundles and `xo replay`.
	Info *Info
	// Warnings are toolchain warnings printed during the build (GoTimersOff).
	Warnings []string
	// Tests lists the test blocks of a native test binary (Mode Test on
	// the llvm or arm64 backend, decision 0083), in their index order.
	Tests []TestInfo
	// Hybrid reports what a hybrid build compiled to assembly.
	Hybrid *HybridInfo
	// Notes are build notes, such as the embedding of the whole zone
	// database (tzdata.go).
	Notes []string
}

// Info identifies a program build (core 11.2): a crash bundle and a trace
// carry it, and `xo replay` rebuilds the program from it.
type Info struct {
	ID       string            `json:"id"`
	Dir      string            `json:"dir"`
	Sources  map[string]string `json:"sources"` // path relative to the line root -> sha256
	Holes    bool              `json:"holes,omitempty"`
	NoMono   bool              `json:"no_mono,omitempty"`
	NoArenas bool              `json:"no_arenas,omitempty"`
	Race     bool              `json:"race,omitempty"`
	TZData   string            `json:"tzdata,omitempty"`
	Release  bool              `json:"release,omitempty"`
	// NoErrorArgs is `--error-args=false` (decision 0092).
	NoErrorArgs bool `json:"no_error_args,omitempty"`
	// Deps are the required module versions the program was built with
	// (decision 0086): "<path>" -> "<version>", or "workspace <dir>" for a
	// module an xo.work overrides (local paths are recorded here only,
	// never in xo.lock).
	Deps map[string]string `json:"deps,omitempty"`
}

// buildInfo computes the build information of a checked program.
func buildInfo(dirs []string, res *check.Result, root string, opts Options) *Info {
	info := &Info{Sources: map[string]string{}, Holes: opts.Holes, NoMono: opts.NoMono, NoArenas: opts.NoArenas, Race: opts.Race, TZData: opts.TZData, Release: opts.release(), NoErrorArgs: opts.release() && opts.NoErrorArgs}
	if len(dirs) > 0 {
		info.Dir, _ = filepath.Abs(dirs[0])
	}
	if bl := res.Deps(); bl != nil && len(bl.Modules) > 0 {
		info.Deps = map[string]string{}
		for _, m := range bl.Modules {
			if m.Workspace {
				info.Deps[m.Path] = "workspace " + m.Dir
			} else {
				info.Deps[m.Path] = m.Version
			}
		}
	}
	for _, m := range res.Info().AllModules() {
		if m.IsStd {
			continue
		}
		for _, f := range m.Files {
			sum := sha256.Sum256(f.Src)
			p := f.Path
			if abs, err := filepath.Abs(p); err == nil {
				if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
					p = rel
				}
			}
			info.Sources[filepath.ToSlash(p)] = hex.EncodeToString(sum[:])
		}
	}
	h := sha256.New()
	var paths []string
	for p := range info.Sources {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(h, "%s %s\n", p, info.Sources[p])
	}
	fmt.Fprintf(h, "holes=%v mono=%v arenas=%v race=%v\n", opts.Holes, !opts.NoMono, !opts.NoArenas, opts.Race)
	if opts.TZData != "" {
		fmt.Fprintf(h, "tzdata=%s\n", opts.TZData)
	}
	if opts.release() {
		fmt.Fprintf(h, "release error_args=%v\n", !opts.NoErrorArgs)
	}
	fs.WalkDir(std.Runtime, "rt", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			data, _ := fs.ReadFile(std.Runtime, p)
			fmt.Fprintf(h, "%s %x\n", p, sha256.Sum256(data))
		}
		return nil
	})
	info.ID = hex.EncodeToString(h.Sum(nil))[:24]
	return info
}

// buildInfoFile is the Go file of package main that hands the build
// information to the runtime.
func buildInfoFile(info *Info) []byte {
	data, _ := json.Marshal(info)
	return []byte("// Code generated by xo. DO NOT EDIT.\n\npackage main\n\nimport rt \"xoprog/rt\"\n\nfunc init() { rt.SetBuildInfo(" + strconv.Quote(string(data)) + ") }\n")
}

// Check runs the checker on dirs and reports whether errors block the
// build. Holes (XO0900 warnings) block it unless holes is set (core 3.7).
func Check(dirs []string, holes bool) (*check.Result, bool, error) {
	return checkOpts(dirs, holes, false)
}

// CheckNative is Check for the native backends: it also loads their
// implementation modules (std/native, decision 0074).
func CheckNative(dirs []string, holes bool) (*check.Result, bool, error) {
	return checkOpts(dirs, holes, true)
}

func checkOpts(dirs []string, holes, native bool) (*check.Result, bool, error) {
	res, err := check.CheckOpts(dirs, check.Options{Native: native})
	if err != nil {
		return nil, false, err
	}
	ok := true
	for _, d := range res.Diagnostics {
		if d.Severity == diag.Error {
			ok = false
		}
		if d.Code == string(diag.Hole) && !holes {
			ok = false
		}
	}
	return res, ok, nil
}

// Build checks and compiles the modules under dirs.
func Build(dirs []string, opts Options) (*Result, error) {
	if opts.NoGC {
		switch opts.Backend {
		case "":
			opts.Backend = NativeBackend()
		case BackendGo:
			return nil, fmt.Errorf("--no-gc needs a native backend (llvm or arm64), not go")
		}
	}
	native := opts.Backend == BackendLLVM || opts.Backend == BackendArm64
	res, ok, err := checkOpts(dirs, opts.Holes, native)
	if err != nil {
		return nil, err
	}
	out := &Result{Diagnostics: res.Diagnostics, Binaries: map[Target]string{}}
	if !ok {
		out.Failed = true
		return out, nil
	}
	if native {
		// the no GC check (decision 0083): everything the native
		// backends cannot lower, reported before any code generation
		if ds := CheckNoGC(dirs, res, opts.Mode); len(ds) > 0 {
			out.Diagnostics = append(out.Diagnostics, ds...)
			diag.Sort(out.Diagnostics)
			out.Failed = true
			return out, nil
		}
	}
	if native && opts.Mode == gen.Test {
		out.Tests = testInfos(dirs, res)
	}
	switch opts.Backend {
	case "", BackendGo:
	case BackendLLVM:
		return buildLLVM(dirs, res, out, opts)
	case BackendArm64:
		return buildArm64(dirs, res, out, opts)
	default:
		return nil, fmt.Errorf("unknown backend %q (want go, llvm, or arm64)", opts.Backend)
	}
	root := lineRoot(dirs)
	var hyb *gen.Hybrid
	if opts.Hybrid && opts.Mode == gen.Build {
		hyb, out.Hybrid = hybridAsm(dirs, root, opts)
	}
	prog, err := gen.Generate(res, gen.Options{Mode: opts.Mode, Holes: opts.Holes, LineRoot: root, NoMono: opts.NoMono, NoRanges: opts.NoRanges, Arenas: !opts.NoArenas, Hybrid: hyb, Release: opts.release(), NoErrorArgs: opts.NoErrorArgs})
	if err != nil {
		return nil, err
	}
	out.Specs = prog.Specs
	if opts.TZData != "" && opts.TZData != "embedded" && opts.TZData != TZDataSystem {
		return nil, fmt.Errorf("unknown --tzdata=%s (want embedded or system)", opts.TZData)
	}
	if out.Notes, err = tzFiles(prog, opts.TZData); err != nil {
		return nil, err
	}
	opts.tags = prog.Tags
	if opts.Mode == gen.Build {
		out.Info = buildInfo(dirs, res, root, opts)
		prog.Files["xo_buildinfo.go"] = buildInfoFile(out.Info)
	}
	var variants []string
	if hyb != nil {
		variants = append(variants, "hybrid")
	}
	if opts.release() {
		variants = append(variants, "release")
		if opts.NoErrorArgs {
			variants = append(variants, "noargs")
		}
	}
	dir, err := buildDir(dirs, opts.Mode, !opts.NoMono, opts.NoRanges, !opts.NoArenas, variants...)
	if err != nil {
		return nil, err
	}
	out.Dir = dir
	if err := writeTree(dir, prog); err != nil {
		return nil, err
	}
	name := progName(dirs)
	if opts.Mode == gen.Test {
		name += ".test"
	}
	out.Name = name
	targets := opts.Targets
	if len(targets) == 0 {
		targets = []Target{Host()}
	}
	timers := needGoTimers(prog)
	for _, t := range targets {
		bin := binPath(dir, opts.Out, name, t, len(targets) > 1)
		warn, err := goBuildTimers(dir, bin, t, opts, timers)
		if err != nil {
			return nil, err
		}
		if warn != "" {
			out.Warnings = append(out.Warnings, warn)
			timers = false
		}
		out.Binaries[t] = bin
	}
	return out, nil
}

// lineRoot is the directory //line paths are relative to: the xo.mod root
// of the first directory, or the directory itself.
func lineRoot(dirs []string) string {
	if len(dirs) == 0 {
		return ""
	}
	abs, err := filepath.Abs(dirs[0])
	if err != nil {
		return ""
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, "xo.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return abs
}

func progName(dirs []string) string {
	if len(dirs) == 0 {
		return "xoprog"
	}
	abs, err := filepath.Abs(dirs[0])
	if err != nil {
		return "xoprog"
	}
	name := filepath.Base(abs)
	if name == "." || name == "/" || name == "" {
		return "xoprog"
	}
	return name
}

func binPath(dir, out, name string, t Target, many bool) string {
	exe := ""
	if t.OS == "windows" {
		exe = ".exe"
	} else if t.OS == "wasip1" {
		exe = ".wasm"
	}
	if many {
		base := fmt.Sprintf("%s-%s-%s%s", name, t.OS, t.Arch, exe)
		if out != "" {
			return filepath.Join(out, base)
		}
		return filepath.Join(dir, "bin", base)
	}
	if out != "" {
		return out
	}
	return filepath.Join(dir, "bin", name+exe)
}

// buildDir is a stable directory per program and mode under the user
// cache, so Go's build cache is reused across builds.
func buildDir(dirs []string, mode gen.Mode, mono, noRanges, arenas bool, variants ...string) (string, error) {
	base := os.Getenv("XO_BUILD_DIR")
	if base == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		base = filepath.Join(cache, "xo", "build")
	}
	h := sha256.New()
	for _, d := range dirs {
		abs, _ := filepath.Abs(d)
		h.Write([]byte(abs + "\x00"))
	}
	fmt.Fprintf(h, "mode=%d", mode)
	if mono {
		fmt.Fprintf(h, " mono")
	}
	if noRanges {
		fmt.Fprintf(h, " noranges")
	}
	if arenas {
		fmt.Fprintf(h, " arenas")
	}
	for _, v := range variants {
		if v != "" {
			fmt.Fprintf(h, " %s", v)
		}
	}
	// layout 2 (2026-10-08) adds package main's xo_adapt.go; older xo
	// binaries do not remove it, so they must not share the directory
	fmt.Fprintf(h, " layout=2")
	dir := filepath.Join(base, hex.EncodeToString(h.Sum(nil))[:16])
	return dir, os.MkdirAll(dir, 0o755)
}

// writeTree writes the generated program and the runtime into dir,
// removing generated packages left from an earlier build.
func writeTree(dir string, prog *gen.Program) error {
	if err := os.RemoveAll(filepath.Join(dir, "m")); err != nil {
		return err
	}
	files := map[string][]byte{}
	for p, src := range prog.Files {
		files[p] = src
	}
	err := fs.WalkDir(std.Runtime, "rt", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "_test.go") {
			return err
		}
		data, err := fs.ReadFile(std.Runtime, p)
		if err != nil {
			return err
		}
		files[p] = data
		return nil
	})
	if err != nil {
		return err
	}
	// remove generated files of package main that no longer exist
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents {
			if _, ok := files[e.Name()]; !ok && !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	// remove runtime files that no longer exist
	if ents, err := os.ReadDir(filepath.Join(dir, "rt")); err == nil {
		for _, e := range ents {
			if _, ok := files["rt/"+e.Name()]; !ok {
				os.Remove(filepath.Join(dir, "rt", e.Name()))
			}
		}
	}
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if old, err := os.ReadFile(full); err == nil && bytes.Equal(old, files[p]) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, files[p], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// BuildError is a failure of the Go toolchain on generated code: a code
// generator bug.
type BuildError struct {
	Target Target
	Output string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("go build for %s failed (code generator bug):\n%s", e.Target, e.Output)
}

// goTimersTags is the build tag list that links the runtime's view of
// pending Go timers (std/rt/gotimers_on.go); tests add a tag that links a
// missing symbol.
var goTimersTags = "xo_gotimers"

// GoTimersOff is the warning printed when a program with Go callbacks
// links without the runtime's view of Go timers.
const GoTimersOff = "xo build: warning: deadlock detection through Go callbacks is off: this Go toolchain does not link runtime.timeSleepUntil (decision 0073)"

// needGoTimers reports whether the program binds a Go package with
// callback adapters, so the runtime may need to see pending Go timers to
// report a deadlock through a Go call. XO_GOTIMERS=off forces it off.
func needGoTimers(prog *gen.Program) bool {
	if os.Getenv("XO_GOTIMERS") == "off" {
		return false
	}
	for p, src := range prog.Files {
		if strings.HasPrefix(p, "rt/xo_gobind_") && bytes.Contains(src, []byte("NewGoCallback(")) {
			return true
		}
	}
	return false
}

// goBuildTimers is goBuild for a program that may need the runtime's
// view of Go timers: it links runtime.timeSleepUntil through a linkname
// (the tag and -checklinkname=0), and when that link fails (a Go release
// without the symbol) builds again without it and warns once. Without
// it, deadlocks through Go calls are never reported (no false reports).
func goBuildTimers(dir, bin string, t Target, opts Options, timers bool) (warn string, err error) {
	if !timers {
		return "", goBuild(dir, bin, t, opts, nil)
	}
	err = goBuild(dir, bin, t, opts, []string{"-tags=" + goTimersTags, "-ldflags=-checklinkname=0"})
	var be *BuildError
	if errors.As(err, &be) && linknameFailed(be.Output) {
		if err := goBuild(dir, bin, t, opts, nil); err != nil {
			return "", err
		}
		fmt.Fprintln(os.Stderr, GoTimersOff)
		return GoTimersOff, nil
	}
	return "", err
}

// linknameFailed reports whether go build output is a failed link of a
// runtime symbol pulled by a linkname.
func linknameFailed(out string) bool {
	return strings.Contains(out, "relocation target runtime.") || strings.Contains(out, "invalid reference to runtime.") ||
		(strings.Contains(out, "undefined") && strings.Contains(out, "runtime."))
}

func goBuild(dir, bin string, t Target, opts Options, extra []string) error {
	// extra: the tag and -checklinkname=0 that let the runtime see
	// pending Go timers (goBuildTimers, std/rt/gotimers_on.go, decision
	// 0073)
	if opts.HybridPure {
		extra = withTag(extra, "xo_pure")
	}
	for _, tag := range opts.tags {
		extra = withTag(extra, tag)
	}
	args := []string{"build", "-trimpath"}
	var ld []string
	for _, a := range extra {
		if s, ok := strings.CutPrefix(a, "-ldflags="); ok {
			ld = append(ld, s)
			continue
		}
		args = append(args, a)
	}
	if !GoDWARF() {
		// no DWARF in the cached objects (a third of each) or in the
		// binary, and no symbol table: Xo's stack traces, faults, crash
		// bundles, and `xo dump` read the pclntab through the Go runtime,
		// which -s -w keep (knowledgebase/infrastructure/toolchain.md,
		// `xo build`)
		args = append(args, "-gcflags=all=-dwarf=false")
		ld = append(ld, "-s", "-w")
	}
	if len(ld) > 0 {
		args = append(args, "-ldflags="+strings.Join(ld, " "))
	}
	if opts.Race {
		args = append(args, "-race")
	}
	args = append(args, ".")
	key := goBinKey(dir, args, t, opts.Race)
	args = append(args[:len(args)-1:len(args)-1], "-o", bin, ".")
	return withBinCache(key, bin, func() error { return goBuildRun(dir, t, opts, args) })
}

// goBuildRun runs go build in dir with args.
func goBuildRun(dir string, t Target, opts Options, args []string) error {
	cmd := exec.Command(goTool(), args...)
	cmd.Dir = dir
	env := append(os.Environ(), "GOOS="+t.OS, "GOARCH="+t.Arch, "GOWORK=off", "GOFLAGS=", "GOCACHE="+GoCacheDir())
	defer trimGoCache()
	if opts.Race {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}
	cmd.Env = env
	var outb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outb, &outb
	lock := lockCacheShared()
	err := cmd.Run()
	lock.unlock()
	if err != nil {
		return &BuildError{Target: t, Output: outb.String()}
	}
	return nil
}

// GoDWARF reports whether Go backend builds keep DWARF and the symbol
// table, for a native debugger such as Delve: XO_GO_DWARF=1. By default
// they are left out.
func GoDWARF() bool { return os.Getenv("XO_GO_DWARF") == "1" }

// goTool finds the go command: $XO_GO, then PATH.
func goTool() string {
	if g := os.Getenv("XO_GO"); g != "" {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return "go"
}
