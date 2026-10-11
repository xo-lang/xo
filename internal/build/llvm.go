package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/llvm"
)

// Backends accepted by Options.Backend.
const (
	BackendGo   = "go"
	BackendLLVM = "llvm"
)

// ErrNoClang reports that the LLVM backend found no clang.
var ErrNoClang = errors.New("the LLVM backend needs clang: install it or set XO_CLANG")

// Clang finds the C compiler of the LLVM backend: $XO_CLANG, then clang
// on PATH.
func Clang() (string, error) {
	if c := os.Getenv("XO_CLANG"); c != "" {
		p, err := exec.LookPath(c)
		if err != nil {
			return "", fmt.Errorf("%w (XO_CLANG=%s: %v)", ErrNoClang, c, err)
		}
		return p, nil
	}
	if p, err := exec.LookPath("clang"); err == nil {
		return p, nil
	}
	return "", ErrNoClang
}

var (
	targetMu    sync.Mutex
	targetAttrs = map[string]string{} // C compiler -> ClangTargetAttrs
	quotedArg   = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

// ClangTargetAttrs returns the LLVM function attributes of the target
// clang compiles C for by default on this host (its -target-cpu and
// -target-feature arguments, from `clang -###`), for the generated IR
// (llvm.Options.TargetAttrs, decision 0119): darwin/arm64 gets apple-m1
// with LSE atomics, Linux on arm64 the generic CPU with outline atomics
// when clang's runtime library has them, as the runtime's C does. ""
// when clang reports neither.
func ClangTargetAttrs(clang string) string {
	targetMu.Lock()
	defer targetMu.Unlock()
	if a, ok := targetAttrs[clang]; ok {
		return a
	}
	out, err := exec.Command(clang, "-###", "-c", "-x", "c", os.DevNull, "-o", os.DevNull).CombinedOutput()
	a := ""
	if err == nil {
		a = parseTargetAttrs(string(out))
	}
	targetAttrs[clang] = a
	return a
}

// parseTargetAttrs reads the target CPU and features from the cc1 line
// of `clang -###` output.
func parseTargetAttrs(out string) string {
	var cpu string
	var feats []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, `"-cc1"`) {
			continue
		}
		var args []string
		for _, m := range quotedArg.FindAllStringSubmatch(line, -1) {
			args = append(args, m[1])
		}
		for i := 0; i+1 < len(args); i++ {
			switch args[i] {
			case "-target-cpu":
				cpu = args[i+1]
			case "-target-feature":
				feats = append(feats, args[i+1])
			}
		}
	}
	var parts []string
	if cpu != "" {
		parts = append(parts, `"target-cpu"="`+cpu+`"`)
	}
	if len(feats) > 0 {
		parts = append(parts, `"target-features"="`+strings.Join(feats, ",")+`"`)
	}
	return strings.Join(parts, " ")
}

// LLVMOptLevel is the clang optimization flag of LLVM builds.
var LLVMOptLevel = "-O2"

// buildLLVM generates LLVM IR for a checked program and compiles it with
// clang and the C runtime (std/llrt) into a host binary.
func buildLLVM(dirs []string, res *check.Result, out *Result, opts Options) (*Result, error) {
	if opts.Mode != gen.Build && opts.Mode != gen.Test {
		return nil, fmt.Errorf("the LLVM backend builds programs and tests only")
	}
	for _, t := range opts.Targets {
		if t != Host() {
			return nil, fmt.Errorf("the LLVM backend builds for the host (%s) only, not %s", Host(), t)
		}
	}
	if opts.Race || opts.Holes {
		return nil, fmt.Errorf("the LLVM backend does not support --race or --holes")
	}
	clang, err := Clang()
	if err != nil {
		return nil, err
	}
	prog, err := llvm.Generate(res, llvm.Options{LineRoot: lineRoot(dirs), ShareProbes: opts.ShareProbes, Tests: opts.Mode == gen.Test, RealTests: opts.RealTests, Release: opts.release(), TargetAttrs: ClangTargetAttrs(clang)})
	if err != nil {
		return nil, err
	}
	dir, err := buildDir(dirs, opts.Mode, false, opts.NoRanges, false, releaseVariant(opts))
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "llvm")
	if opts.ShareProbes {
		dir += "-share"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out.Dir = dir
	ll := filepath.Join(dir, "prog.ll")
	if err := os.WriteFile(ll, prog.IR, 0o644); err != nil {
		return nil, err
	}
	rt, err := writeRuntime(dir)
	if err != nil {
		return nil, err
	}
	name := progName(dirs)
	if opts.Mode == gen.Test {
		name += ".test"
	}
	out.Name = name
	bin := binPath(dir, opts.Out, name, Host(), false)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return nil, err
	}
	flags, tzsrcs, notes, err := timeFlags(dir, res, prog.TZ, opts)
	if err != nil {
		return nil, err
	}
	flags = append(flags, IOFlags(prog.IR)...)
	out.Notes = notes
	srcs := append([]string{ll, rt}, tzsrcs...)
	csrcs, cflags, err := cInputs(res, dir)
	if err != nil {
		return nil, err
	}
	srcs = append(srcs, csrcs...)
	flags = append(flags, cflags...)
	key := nativeBinKey(dir, clang, append([]string{"llvm", LLVMOptLevel}, flags...), srcs)
	if err := withBinCache(key, bin, func() error { return clangBuild(clang, srcs, bin, flags...) }); err != nil {
		return nil, err
	}
	out.Binaries[Host()] = bin
	return out, nil
}

// ClangError is a failure of clang on generated IR: a backend bug, or a
// broken C toolchain.
type ClangError struct {
	Output string
}

func (e *ClangError) Error() string {
	msg := "clang failed on the generated LLVM IR:\n" + e.Output
	if missingSanitizerRuntime(e.Output) {
		msg += "\nhint: clang is installed without its compiler-rt sanitizer runtimes " +
			"(Ubuntu: apt-get install libclang-rt-<clang major>-dev; Debian: libclang-rt-dev), " +
			"see knowledgebase/gotchas/ubuntu-clang-without-sanitizer-runtimes.md"
	}
	return msg
}

// missingSanitizerRuntime reports whether clang's output says the linker
// could not find a compiler-rt archive (libclang_rt.tsan, .asan, ...):
// Ubuntu's clang package does not depend on them.
func missingSanitizerRuntime(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "libclang_rt.") &&
			(strings.Contains(line, "cannot find") || strings.Contains(line, "No such file")) {
			return true
		}
	}
	return false
}

var (
	sysrootMu     sync.Mutex
	sysrootChosen bool
	sysroot       string // "" means clang's default
)

// sysrootCandidates lists macOS SDKs to try when linking with clang's
// default SDK fails: on 2026-10-08 the Command Line Tools SDK
// (MacOSX27.0) has a libSystem.tbd the Xcode 26 linker rejects
// ("unknown architecture arm64e.x1"), see knowledgebase/gotchas.
func sysrootCandidates() []string {
	if s := os.Getenv("XO_SYSROOT"); s != "" {
		return []string{s}
	}
	if runtime.GOOS != "darwin" {
		return []string{""}
	}
	out := []string{""}
	if p, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
		out = append(out, strings.TrimSpace(string(p)))
	}
	out = append(out, "/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk")
	if ms, _ := filepath.Glob("/Library/Developer/CommandLineTools/SDKs/MacOSX*.sdk"); len(ms) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(ms)))
		out = append(out, ms...)
	}
	return out
}

// sysrootMemo is the file remembering the SDK that linked last time, so
// each xo process does not retry a broken default first.
func sysrootMemo() string {
	if os.Getenv("XO_SYSROOT") != "" {
		return ""
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "xo", "llvm-sysroot")
}

// CompileC compiles and links C and LLVM IR sources into bin with clang
// and flags, choosing a macOS SDK as LLVM builds do (the runtime's thread
// tests: ThreadSanitizer builds, decision 0058).
func CompileC(flags, srcs []string, bin string) error {
	clang, err := Clang()
	if err != nil {
		return err
	}
	return ccBuild(clang, flags, srcs, bin)
}

// clangBuild compiles and links LLVM IR and C sources into bin.
func clangBuild(clang string, srcs []string, bin string, flags ...string) error {
	return ccBuild(clang, append([]string{LLVMOptLevel, "-flto", "-Wno-override-module"}, flags...), srcs, bin)
}

// ccBuild compiles and links srcs into bin with the C compiler cc and
// flags, finding a working macOS SDK on the first build of the process.
func ccBuild(clang string, flags, srcs []string, bin string) error {
	tls := usesTLS(srcs)
	// libraries of `use c` (decision 0116) go after the objects
	flags, libs := splitLibs(flags)
	try := func(root string) (string, error) {
		// the C runtime raises faults through the frames of generated
		// code and its own (decision 0085): its functions may unwind, and
		// every frame needs unwind tables
		args := append(append([]string(nil), flags...), "-fexceptions", "-funwind-tables", "-o", bin)
		if root != "" {
			args = append(args, "-isysroot", root)
		}
		var tlsCompile, tlsLink []string
		if tls {
			// std/http's TLS (decision 0105)
			c, l, err := tlsArgs(clang, flags, root)
			if err != nil {
				return err.Error(), err
			}
			tlsCompile, tlsLink = c, l
			args = append(args, c...)
		}
		// the C runtime compiled once per flags and sources (rtobj.go)
		srcs := srcs
		if i := runtimeSource(srcs, flags); i >= 0 {
			obj, out, err := runtimeObject(clang, flags, root, tlsCompile, srcs[i])
			if err != nil {
				return out, err
			}
			if obj != "" {
				srcs = append(append(append([]string(nil), srcs[:i]...), obj), srcs[i+1:]...)
			}
		}
		args = append(args, srcs...)
		args = append(args, tlsLink...)
		args = append(args, libs...)
		args = append(args, "-lm", "-lpthread")
		cmd := exec.Command(clang, args...)
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		err := cmd.Run()
		return b.String(), err
	}
	sysrootMu.Lock()
	chosen, known := sysrootChosen, sysroot
	sysrootMu.Unlock()
	if chosen {
		out, err := try(known)
		if err != nil {
			return &ClangError{Output: out}
		}
		return nil
	}
	var first string
	cands := sysrootCandidates()
	memo := sysrootMemo()
	if memo != "" {
		if data, err := os.ReadFile(memo); err == nil {
			cands = append([]string{strings.TrimSpace(string(data))}, cands...)
		}
	}
	for _, root := range cands {
		out, err := try(root)
		if err == nil {
			sysrootMu.Lock()
			sysrootChosen, sysroot = true, root
			sysrootMu.Unlock()
			if memo != "" {
				os.MkdirAll(filepath.Dir(memo), 0o755)
				os.WriteFile(memo, []byte(root+"\n"), 0o644)
			}
			return nil
		}
		// report an SDK independent failure over an SDK mismatch
		if first == "" || (strings.Contains(first, "tapi error") && !strings.Contains(out, "tapi error")) {
			first = out
		}
		// only a link failure is worth another SDK
		if !strings.Contains(out, "linker command failed") {
			return &ClangError{Output: out}
		}
	}
	return &ClangError{Output: first}
}
