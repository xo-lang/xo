package xotool

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The clang cache (2026-10-18): xo compiles and links the LLVM backend's
// IR with clang itself, so CompileC's cache does not see it, and a full
// internal/llvm run spent most of its 50 minutes in clang at -O2 with
// LTO, on IR most changes leave alone (an arm64 emitter change, a test
// change). EnableTestClangCache points $XO_CLANG at clangcache, a small
// command that runs the real clang ($XO_CLANG_CACHED) through
// RunCachedClang: each call that writes an output (-o) from IR, objects,
// and archives is keyed on clang and its version, the target,
// $XO_SYSROOT, its arguments, and the contents of every input file (not
// their paths: xo's build directories differ per run), and a hit copies
// the stored output. C, assembly, and preprocessor runs are not cached.
//
// ld64 records the -object_path_lto object (full LTO's lto.o, thin
// LTO's directory) by path and modification time in the binary, for
// dsymutil to read after the link (compiler/xo link.xo). A cached link
// writes it inside the cache entry instead of xo's build directory, so
// the path a later hit's binary names still exists, unchanged; the
// entry is removed with it (XO_CACHE_MAX_AGE, trimCCCache).

const clangRealEnv = "XO_CLANG_CACHED"

// EnableTestClangCache builds clangcache and makes it xo's clang for the
// test binary (TestMain), with the real clang in $XO_CLANG_CACHED, when
// the C compile cache is on (EnableTestCCache) and XO_CLANG_CACHE is not
// "off". The returned function removes the build; call it after m.Run.
func EnableTestClangCache() (func(), error) {
	none := func() {}
	if ccCacheDir() == "" || os.Getenv("XO_CLANG_CACHE") == "off" || os.Getenv(clangRealEnv) != "" {
		return none, nil
	}
	real, err := Clang()
	if err != nil {
		return none, nil // the tests that need clang skip
	}
	dir, err := os.MkdirTemp("", "xo-clangcache-")
	if err != nil {
		return none, err
	}
	bin := filepath.Join(dir, "clang")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/xo-lang/xo/internal/xotool/clangcache")
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return none, fmt.Errorf("building clangcache: %v\n%s", err, out)
	}
	os.Setenv(clangRealEnv, real)
	os.Setenv("XO_CLANG", bin)
	return func() { os.RemoveAll(dir) }, nil
}

// RunCachedClang runs clang real with args through the clang cache (see
// above) and returns its exit status.
func RunCachedClang(real string, args []string) int {
	if real == "" {
		fmt.Fprintln(os.Stderr, "clangcache: $"+clangRealEnv+" is not set")
		return 2
	}
	dir := ccCacheDir()
	key, out, lto, ok := clangKey(real, args)
	if dir == "" || !ok {
		return runClang(real, args)
	}
	entry := filepath.Join(dir, "clang-"+key)
	stored := filepath.Join(entry, "out")
	if isFile(filepath.Join(entry, "done")) {
		if err := copyExe(stored, out); err == nil {
			now := time.Now()
			os.Chtimes(entry, now, now)
			return 0
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runClang(real, args)
	}
	if err := os.Mkdir(entry, 0o755); err != nil {
		// another clang is writing this entry (or one stopped half way,
		// until trimCCCache removes it): uncached
		return runClang(real, args)
	}
	run := args
	if lto != "" {
		run = make([]string, len(args))
		for i, a := range args {
			if a == lto {
				p := strings.TrimPrefix(a, ltoFlag)
				a = ltoFlag + filepath.Join(entry, filepath.Base(p))
			}
			run[i] = a
		}
	}
	if rc := runClang(real, run); rc != 0 {
		os.RemoveAll(entry)
		return rc
	}
	if err := copyExe(out, stored); err != nil {
		os.RemoveAll(entry)
		return 0
	}
	os.WriteFile(filepath.Join(entry, "done"), nil, 0o644)
	return 0
}

const ltoFlag = "-Wl,-object_path_lto,"

// clangKey is the cache key of a clang call, its output, and its
// -object_path_lto argument; ok is false for a call the cache leaves
// alone (no output, a C or assembly source, the preprocessor).
func clangKey(real string, args []string) (key, out, lto string, ok bool) {
	h := sha256.New()
	fmt.Fprintf(h, "clang1\n%s\n%s\n%s/%s\n%s\n", real, ccVersion(real), runtime.GOOS, runtime.GOARCH, os.Getenv("XO_SYSROOT"))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o" && i+1 < len(args):
			out = args[i+1]
			i++
			fmt.Fprintf(h, "-o %s\n", filepath.Ext(out))
			continue
		case strings.HasPrefix(a, ltoFlag):
			lto = a
			fmt.Fprintf(h, "lto %s\n", filepath.Base(strings.TrimPrefix(a, ltoFlag)))
			continue
		case a == "-###" || a == "-E" || a == "-S" || strings.HasPrefix(a, "-M") || a == "-":
			return "", "", "", false
		}
		if st, err := os.Stat(a); err == nil && st.Mode().IsRegular() {
			switch filepath.Ext(a) {
			case ".ll", ".bc", ".o", ".a":
			default:
				return "", "", "", false
			}
			data, err := os.ReadFile(a)
			if err != nil {
				return "", "", "", false
			}
			fmt.Fprintf(h, "file %s %d\n", filepath.Ext(a), len(data))
			h.Write(data)
			continue
		}
		fmt.Fprintf(h, "arg %q\n", a)
	}
	if out == "" || out == os.DevNull {
		return "", "", "", false
	}
	return hex.EncodeToString(h.Sum(nil))[:32], out, lto, true
}

// runClang runs clang with args, its output this process's, and returns
// its exit status.
func runClang(real string, args []string) int {
	cmd := exec.Command(real, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "clangcache:", err)
		return 2
	}
	return 0
}
