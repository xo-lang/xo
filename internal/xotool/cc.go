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
	for _, sym := range []string{"xo_fs_stat", "xo_fs_names", "xo_fs_write", "xo_fs_rename", "xo_fs_remove", "xo_fs_mkdir_all", "xo_fs_chtimes", "xo_fs_create_temp", "xo_fs_cwd", "xo_fs_case_sensitive", "xo_fs_canceled", "xo_fs_info", "xo_time_of", "xo_time_ns",
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
	// and of the leaf text of Display and Debug (decision 0193 step 5,
	// XO_LLRT_C=show)
	for _, sym := range []string{"xo_rs_append_u64", "xo_rs_append_rune_quote"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_SHOW_C")
			break
		}
	}
	// and of the report of a property test's case (decision 0193 step 5,
	// XO_LLRT_C=report)
	if callsSym(code, "xo_prop_report") {
		out = append(out, "-DXO_REPORT_C")
	}
	// and of the zone rules (decision 0193 step 5, XO_LLRT_C=tz)
	for _, sym := range []string{"xo_tz_load", "xo_tz_offset", "xo_tz_abbr"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_TZ_C")
			break
		}
	}
	// and of Atomic's operations (decision 0200, XO_LLRT_C=atomic); not
	// xo_atomic_new, which the Xo path calls too
	for _, sym := range []string{"xo_atomic_get", "xo_atomic_set", "xo_atomic_swap", "xo_atomic_cas", "xo_atomic_add", "xo_atomic_max"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_ATOMIC_C")
			break
		}
	}
	// and of the Fs path policy (decision 0193 step 6, XO_LLRT_C=fspath)
	for _, sym := range []string{"xo_fs_mkdir_all", "xo_fs_remove_all", "xo_fs_create_temp", "xo_fs_cwd"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_FSPATH_C")
			break
		}
	}
	// and of the scalar values of property tests (decision 0195,
	// XO_LLRT_C=gen; xo_gen_size also names xo_gen_sized)
	for _, sym := range []string{"xo_gen_size", "xo_gen_any_int", "xo_gen_float", "xo_gen_rune", "xo_gen_str", "xo_gen_duration", "xo_gen_time"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_GEN_C")
			break
		}
	}
	// and of MemFs.fail_next's pattern match (decision 0195,
	// XO_LLRT_C=glob)
	if callsSym(code, "xo_test_glob") {
		out = append(out, "-DXO_GLOB_C")
	}
	// and of the hashes of std/crypto (decision 0195, XO_LLRT_C=digest)
	for _, sym := range []string{"xo_crypto_sha256", "xo_crypto_hmac_sha256", "xo_crypto_pbkdf2", "xo_crypto_argon2id", "xo_crypto_b64", "xo_crypto_verify_password"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_DIGEST_C")
			break
		}
	}
	// and of the integer operations of generated code and the intrinsics
	// (decision 0195, XO_LLRT_C=ints)
	for _, sym := range []string{"xo_int_op", "xo_int_conv", "xo_int_in_range", "xo_rune_of", "xo_rune_valid", "xo_floor_div", "xo_str_hash",
		// the C versions of the intrinsics (a non-constant op of int_wrap)
		"xo_int_wrap", "xo_u64_mul_hi", "xo_float_bits", "xo_float_from_bits", "xo_bytes_word", "xo_int_fits", "xo_u64_to_float", "xo_f32_round", "xo_i64_to_f32"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_INTS_C")
			break
		}
	}
	// and of the reference counts (decision 0207 stage 2, XO_LLRT_C=rc)
	for _, sym := range []string{"xo_rc_inc_c", "xo_rc_dec_c", "xo_rc_dec_shared_c", "xo_rc_claim_c", "xo_rc_mark_c"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_RC_C")
			break
		}
	}
	// and of the heap (decision 0207 stage 3, XO_LLRT_C=alloc)
	for _, sym := range []string{"xo_alloc_new_c", "xo_alloc_free_c", "xo_alloc_refill_c", "xo_alloc_free_mt_c", "xo_alloc_resize_c"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_ALLOC_C")
			break
		}
	}
	// and of the scheduler core (decision 0212, XO_LLRT_C=sched)
	if callsSym(code, "xo_sched_c") {
		out = append(out, "-DXO_SCHED_C")
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
// the SDKs of sysrootCandidates until one links. It does not link the
// TLS library: a program using std/http's TLS is built with Build.
func CompileC(flags, srcs []string, bin string) error {
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
