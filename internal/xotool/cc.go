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

// IOFlags returns the C compiler flags that compile the runtime's Fs
// operations (std/llrt/xo_fs.c, XO_FS) and standard input reads
// (xo_stdin.c, XO_STDIN), and Proc.run (xo_proc.c, XO_PROC) into a
// native program whose code calls them
// (decision 0102), so programs without them do not grow.
func IOFlags(code []byte) []string {
	var out []string
	for _, sym := range []string{"xo_fs_stat", "xo_fs_names", "xo_fs_write", "xo_fs_rename", "xo_fs_remove", "xo_fs_chtimes", "xo_fs_case_sensitive", "xo_fs_canceled", "xo_fs_info", "xo_time_of", "xo_time_ns",
		// handles, copies, modes, links, locks (decision 0172)
		"xo_fs_open", "xo_fs_file_", "xo_fs_copy", "xo_fs_perm", "xo_fs_chmod", "xo_fs_symlink", "xo_fs_read_link", "xo_fs_lock", "xo_fs_unlock",
		// the system calls under std/native/llrt/fspath.xo (decision 0193 step 6)
		"xo_fs_raw_"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_FS")
			break
		}
	}
	for _, sym := range []string{"xo_stdin_", "xo_tw_set_input", "xo_tw_set_tty", "xo_isatty", "xo_term_width"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_STDIN")
			break
		}
	}
	if callsSym(code, "xo_proc_") {
		// Proc.run and FakeProc.on_run (xo_proc.c, decision 0107)
		out = append(out, "-DXO_PROC")
	}
	// the C path of the number and time text (decision 0193), which only
	// XO_LLRT_C=text builds call: no pack has it, so the runtime is
	// compiled locally
	for _, sym := range []string{"xo_int_parse", "xo_rs_append_duration", "xo_rs_append_time", "xo_rs_append_quote", "xo_duration_parse", "xo_time_parse"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_TEXT_C")
			break
		}
	}
	// Decision 0198 deleted the C paths of show, report, tz, atomic,
	// fspath, gen, glob, digest, ints (not the intrinsics), rc, and alloc
	// with their -D flags, and no C defines xo_fs_mkdir_all,
	// xo_fs_create_temp, or xo_fs_cwd any more.
	// The C versions of the intrinsics (a non-constant op of int_wrap;
	// decision 0195)
	for _, sym := range []string{"xo_int_wrap", "xo_u64_mul_hi", "xo_float_bits", "xo_float_from_bits", "xo_bytes_word", "xo_int_fits", "xo_u64_to_float", "xo_f32_round", "xo_i64_to_f32"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_INTS_C")
			break
		}
	}
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

// CompileC compiles and links C and LLVM IR sources into bin with clang
// ($XO_CLANG, else clang) and flags, with unwind tables (the runtime
// raises faults through every frame, decision 0085), the -l flags after
// the sources, and -lm -lpthread (or lld on Windows); on macOS it tries
// the SDKs of sysrootCandidates until one links. When generated code in
// srcs calls std/http's runtime it also compiles with the TLS library's
// flags and links the library (tls.go), built for the sanitizers of flags.
func CompileC(flags, srcs []string, bin string) error {
	clang, err := Clang()
	if err != nil {
		return err
	}
	tls := usesTLS(srcs)
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
		var tlsLink []string
		if tls {
			// std/http's TLS (decision 0105)
			c, l, err := tlsArgs(clang, other, root)
			if err != nil {
				return err.Error(), err
			}
			args = append(args, c...)
			tlsLink = l
		}
		args = append(args, srcs...)
		args = append(args, tlsLink...)
		args = append(args, libs...)
		if runtime.GOOS == "windows" {
			args = append(args, "-fuse-ld=lld", "-Wl,/Brepro")
		} else {
			args = append(args, "-lm", "-lpthread")
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
