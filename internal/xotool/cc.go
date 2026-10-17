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
// directly (decision 0215 copied them from the Go build package): a
// clang compile and link that finds a working macOS SDK. xo (compiler/xo
// clang.xo) has its own.

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

// FrameworkFlags is compiler/xo link.xo framework_args: the Security and
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

// WeakUndefinedFlags is compiler/xo link.xo weak_undefined_args: the
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
