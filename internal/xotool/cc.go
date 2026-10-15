package xotool

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// The C side of the native backends that the runtime tests drive
// directly (decision 0215 copied them from the Go build package): the
// runtime's I/O flags for a program's code, and a clang compile and link
// that finds a working macOS SDK. xo (selfhost/xo link.xo io_flags and
// clang.xo) has its own; TestIOFlagsMatchXo keeps the tables the same.

// IOFlags returns the C compiler flags that compile the runtime's
// standard input reads (XO_STDIN), and Proc.run (xo_proc.c, XO_PROC) into a
// native program whose code calls them
// (decision 0102), so programs without them do not grow.
func IOFlags(code []byte) []string {
	var out []string
	for _, sym := range []string{"xo_stdin_"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_STDIN")
			break
		}
	}
	if callsSym(code, "xo_proc_") {
		// Proc.run and FakeProc.on_run (xo_proc.c, decision 0107)
		out = append(out, "-DXO_PROC")
	}
	// Decision 0198 deleted the C paths of show, report, tz, atomic,
	// fspath, gen, glob, digest, ints (not the intrinsics), rc, and alloc
	// with their -D flags, and no C defines xo_fs_mkdir_all,
	// xo_fs_create_temp, or xo_fs_cwd any more; decision 0227 deleted
	// the C path of text (-DXO_TEXT_C); the C Fs operations (-DXO_FS)
	// and the C intrinsics (-DXO_INTS_C) went with the 0.12.0 seed.
	return out
}

// callsSym reports whether code (arm64 assembly or LLVM IR) refers to the
// runtime symbol sym: preceded by what a reference has before it (`bl
// xo_f`, `bl _xo_f`, `@xo_f`, `:lo12:xo_f`, an operand after a comma or
// tab), not by the quote of a string literal (`.ascii "xo_f"`, `c"xo_f"`),
// which is the program's data. The self hosted compiler's own tables of
// these names (selfhost/xo link.xo io_flags, which does the same) turned
// on -DXO_UNICODE_C (decision 0180's C path, deleted by decision 0198)
// for xo1, which no pack variant has
// (gotchas/runtime-flags-from-string-literals.md).
func callsSym(code []byte, sym string) bool {
	for _, p := range symRefPrefixes {
		if bytes.Contains(code, []byte(p+sym)) {
			return true
		}
	}
	return false
}

var symRefPrefixes = []string{" ", "\t", "_", "@", ":", ","}

var (
	sysrootMu     sync.Mutex
	sysrootChosen bool
	sysroot       string
)

// sysrootCandidates lists the macOS SDKs to try when linking with
// clang's default SDK fails ($XO_SYSROOT alone when set).
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

// ClangError is a failed clang compile or link and its output.
type ClangError struct{ Output string }

func (e *ClangError) Error() string { return "clang: " + e.Output }

// compileC (CompileC without its cache) compiles and links C and LLVM IR sources into bin with clang
// ($XO_CLANG, else clang) and flags, with unwind tables (the runtime
// raises faults through every frame, decision 0085), the -l flags after
// the sources, and -lm -lpthread (or lld on Windows); on macOS it tries
// the SDKs of sysrootCandidates until one links.
func compileC(flags, srcs []string, bin string) error {
	clang, err := Clang()
	if err != nil {
		return err
	}
	var other, libs []string
	for _, f := range flags {
		if strings.HasPrefix(f, "-l") && len(f) > 2 {
			libs = append(libs, f)
		} else {
			other = append(other, f)
		}
	}
	try := func(root string) (string, error) {
		args := append(append([]string(nil), other...), "-fexceptions", "-funwind-tables", "-o", bin)
		if root != "" {
			args = append(args, "-isysroot", root)
		}
		args = append(args, srcs...)
		args = append(args, libs...)
		if runtime.GOOS == "windows" {
			args = append(args, "-fuse-ld=lld", "-Wl,/Brepro")
		} else {
			args = append(args, "-lm", "-lpthread")
		}
		if runtime.GOOS == "darwin" {
			args = append(args, WeakUndefinedFlags(srcs)...)
			args = append(args, FrameworkFlags(srcs)...)
		}
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
		if out, err := try(known); err != nil {
			return &ClangError{Output: out}
		}
		return nil
	}
	first := ""
	for _, root := range sysrootCandidates() {
		out, err := try(root)
		if err == nil {
			sysrootMu.Lock()
			sysrootChosen, sysroot = true, root
			sysrootMu.Unlock()
			return nil
		}
		if first == "" {
			first = out
		}
		// only a link failure is worth another SDK
		if !strings.Contains(out, "linker command failed") {
			return &ClangError{Output: out}
		}
	}
	if first == "" {
		return errors.New("clang: no SDK to try")
	}
	return &ClangError{Output: first}
}

// FrameworkFlags is selfhost/xo link.xo framework_args: the Security and
// CoreFoundation frameworks when a generated source calls the framework
// (std/native/sys_darwin.xo, tlscert.xo apple_trust).
func FrameworkFlags(srcs []string) []string {
	mark := []byte("SecTrust" + "EvaluateWithError")
	for _, src := range srcs {
		if !strings.HasSuffix(src, ".ll") && !strings.HasSuffix(src, ".s") {
			continue
		}
		if b, err := os.ReadFile(src); err == nil && bytes.Contains(b, mark) {
			return []string{"-framework", "Security", "-framework", "CoreFoundation"}
		}
	}
	return nil
}

// WeakUndefinedFlags is selfhost/xo link.xo weak_undefined_args: the
// weak references of the LLVM IR or assembly sources (decision 0232:
// `declare extern_weak ... @sym(`, `.weak_reference _sym`) as macOS ld's
// -U flags, which leave a symbol no image defines 0.
func WeakUndefinedFlags(srcs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, src := range srcs {
		if !strings.HasSuffix(src, ".ll") && !strings.HasSuffix(src, ".s") {
			continue
		}
		b, err := os.ReadFile(src)
		if err != nil || (!bytes.Contains(b, []byte("extern_weak")) && !bytes.Contains(b, []byte(".weak_reference"))) {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			l := strings.TrimSpace(line)
			sym := ""
			if rest, ok := strings.CutPrefix(l, ".weak_reference "); ok {
				sym = strings.TrimSpace(rest)
			} else if strings.HasPrefix(l, "declare extern_weak ") {
				if _, at, ok := strings.Cut(l, "@"); ok {
					name, _, _ := strings.Cut(at, "(")
					sym = "_" + name
				}
			}
			if sym != "" && !seen[sym] {
				seen[sym] = true
				out = append(out, "-Wl,-U,"+sym)
			}
		}
	}
	return out
}
