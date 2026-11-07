package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestCrashReport: a hardware fault (decision 0343) is reported on
// standard error by the runtime's crash handler (std/native/llrt
// crash.xo): the signal, the address, the registers, and the frames
// named from the program's DWARF, the faulting Xo frame's line first
// after the C function's; then the signal's default action ends the
// program, so its status is the signal's as before. XO_CRASH_REPORT=0
// leaves the old behavior: no report, the same signal.
func TestCrashReport(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	for _, be := range tracebackBackends(t) {
		t.Run(be, func(t *testing.T) { testCrashReport(t, be) })
	}
}

func testCrashReport(t *testing.T, be string) {
	res, err := xotool.Build([]string{filepath.Join("testdata", "crash")}, xotool.Options{Backend: be})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	run := func(env ...string) (string, string, syscall.Signal) {
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Env = append(os.Environ(), env...)
		var outb, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &outb, &errb
		_ = cmd.Run()
		ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if !ok || !ws.Signaled() {
			t.Fatalf("%v: want a signal, got %v\n%s", env, cmd.ProcessState, errb.String())
		}
		return outb.String(), errb.String(), ws.Signal()
	}
	out, got, sig := run("XO_CRASH_KIND=segv")
	if out != "before\n" || sig != syscall.SIGSEGV {
		t.Fatalf("segv: stdout %q, signal %v\n%s", out, sig, got)
	}
	for _, want := range []string{
		"fatal error: SIGSEGV: segmentation violation (code ",
		") at address 0x10\n",
		"\nregisters:\n",
		"\nframes: 0x",
		"\ntraceback:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("segv: the report lacks %q:\n%s", want, got)
		}
	}
	// the C function (LLVM names it with its module's prefix; its file
	// and line from the C unit's debug information, decision 0446), then
	// poke at its call (the arm's line), the recursion, main
	// (an LLVM build keeps the adapter's xo_c_ thunk as a frame of its
	// own, with its line, between the C function and poke; Linux's LTO
	// inlines it into poke and the frame says [inlined])
	const loc = `( \([^)]*\))?`
	const thunk = `(  xo_c_\S+` + loc + `( \[inlined\])?\n)?`
	frames := regexp.MustCompile(`(?m)^  \S*crashc_write` + loc + `\n` + thunk + `  poke \(main\.xo:18\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n  main \(main\.xo:55\)$`)
	if !frames.MatchString(got) {
		t.Errorf("segv: the traceback lacks the frames:\n%s", got)
	}
	if strings.Contains(got, "crash report faulted") {
		t.Errorf("segv: the handler ran twice:\n%s", got)
	}
	// the build information's line (decision 0354 stage 2)
	if !regexp.MustCompile(`\nbuild: xo \S+, \w+, \w+/\w+, id [0-9a-f]{24}, debug \(debug-contracts=on dbg=on error-args=on debug-info=\w+ profile-support=on trace-support=on( opt=O\d)?\)\npc 0x`).MatchString(got) {
		t.Errorf("segv: the report lacks the build line:\n%s", got)
	}
	if !strings.HasPrefix(got, "fatal error: SIGSEGV") {
		t.Errorf("segv: the report does not start with its first line:\n%s", got)
	}
	// xo dev symbolize (stage 3): the same traceback from the report's
	// frames and the binary alone
	symbolize(t, bin, got)
	// a C function the link may inline into poke (crashc_write_inl has no
	// noinline): with the C unit's debug information (decision 0446) the
	// report shows it either as its own frame with its file and line, or
	// as an inlined frame of poke; a Linux LLVM link's full LTO inlines
	// it (gotchas/linux-lto-inlines-c-frames.md), so there the inlined
	// form is required
	_, got, sig = run("XO_CRASH_KIND=inl")
	inl := regexp.MustCompile(`(?m)^  \S*crashc_write_inl \(crashc\.c:\d+\)( \[inlined\])?\n` + thunk + `  poke \(main\.xo:17\)\n  poke \(main\.xo:10\)$`)
	if sig != syscall.SIGSEGV || !inl.MatchString(got) {
		t.Errorf("inl: signal %v, the traceback lacks the C frame with its line:\n%s", sig, got)
	}
	if runtime.GOOS == "linux" && be == "llvm" && !strings.Contains(got, "[inlined]") {
		t.Errorf("inl: the Linux LLVM link inlines crashc_write_inl, want it as an inlined frame:\n%s", got)
	}
	// the trap instruction: brk on arm64 (SIGTRAP)
	_, got, sig = run("XO_CRASH_KIND=trap")
	if sig != syscall.SIGTRAP || !strings.HasPrefix(got, "fatal error: SIGTRAP: trace trap (code ") || strings.Contains(got, "crash report faulted") || !regexp.MustCompile(`crashc_trap`+loc+`\n`+thunk+`  poke \(main\.xo:15\)\n`).MatchString(got) {
		t.Errorf("trap: signal %v\n%s", sig, got)
	}
	// a stack overflow in C: reported on the alternate signal stack, named
	// a stack overflow before the signal's line (a guard page hit is
	// SIGBUS on macOS, SIGSEGV on Linux)
	_, got, sig = run("XO_CRASH_KIND=overflow")
	if (sig != syscall.SIGSEGV && sig != syscall.SIGBUS) || !strings.HasPrefix(got, "fatal error: stack overflow\nfatal error: SIG") || !strings.Contains(got, "crashc_deep") {
		t.Errorf("overflow: signal %v\n%s", sig, got)
	}
	// unbounded Xo recursion on main's stack and on a task's
	for _, k := range []string{"xdeep", "xdeep_task"} {
		_, got, sig = run("XO_CRASH_KIND=" + k)
		if (sig != syscall.SIGSEGV && sig != syscall.SIGBUS) || !strings.HasPrefix(got, "fatal error: stack overflow\nfatal error: SIG") || !strings.Contains(got, "xdeep (main.xo:") {
			t.Errorf("%s: signal %v\n%s", k, sig, got)
		}
	}
	// a fault that is not on a guard page is not a stack overflow
	if _, got, _ = run("XO_CRASH_KIND=segv"); strings.Contains(got, "stack overflow") {
		t.Errorf("segv: reported as a stack overflow:\n%s", got)
	}
	// a fault in a task (a worker's fiber): its own frames
	_, got, sig = run("XO_CRASH_KIND=task")
	if sig != syscall.SIGSEGV || !strings.HasPrefix(got, "fatal error: SIGSEGV") || !regexp.MustCompile(`crashc_write`+loc+`\n`+thunk+`  poke \(main\.xo:18\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n`).MatchString(got) {
		t.Errorf("task: signal %v\n%s", sig, got)
	}
	// XO_CRASH_SLEEP=1: the process names its pid and holds a second after
	// phase 1 (for vmmap), then ends with the same signal; unset, it does not
	t0 := time.Now()
	_, got, sig = run("XO_CRASH_KIND=segv", "XO_CRASH_SLEEP=1")
	held := time.Since(t0)
	if sig != syscall.SIGSEGV || !regexp.MustCompile(`(?m)^crash: holding for 1 s, pid [0-9]+$`).MatchString(got) || held < 900*time.Millisecond {
		t.Errorf("XO_CRASH_SLEEP=1: signal %v, held %v, stderr:\n%s", sig, held, got)
	}
	t0 = time.Now()
	_, got, _ = run("XO_CRASH_KIND=segv")
	if strings.Contains(got, "crash: holding") || time.Since(t0) > 900*time.Millisecond {
		t.Errorf("no XO_CRASH_SLEEP: held %v, stderr:\n%s", time.Since(t0), got)
	}
	// XO_CRASH_FILE (decision 0520): the same report appended to the file
	// as well, both phases; a second crash appends to it
	cf := filepath.Join(t.TempDir(), "crash.txt")
	_, got, sig = run("XO_CRASH_KIND=segv", "XO_CRASH_FILE="+cf)
	if b, err := os.ReadFile(cf); err != nil || sig != syscall.SIGSEGV || string(b) != got || !strings.Contains(got, "\ntraceback:\n") {
		t.Errorf("XO_CRASH_FILE: signal %v, %v; the file:\n%s\nstderr:\n%s", sig, err, b, got)
	}
	_, got2, _ := run("XO_CRASH_KIND=trap", "XO_CRASH_FILE="+cf)
	if b, _ := os.ReadFile(cf); string(b) != got+got2 {
		t.Errorf("XO_CRASH_FILE: a second report is not appended:\n%s", b)
	}
	// off: no report, the same signal
	_, got, sig = run("XO_CRASH_KIND=segv", "XO_CRASH_REPORT=0")
	if got != "" || sig != syscall.SIGSEGV {
		t.Errorf("XO_CRASH_REPORT=0: signal %v, stderr:\n%s", sig, got)
	}
}

// symbolize checks that `xo dev symbolize BIN` names report's frames as
// the report's own traceback does.
func symbolize(t *testing.T, bin, report string) {
	t.Helper()
	i := strings.Index(report, "traceback:\n")
	if i < 0 {
		t.Errorf("symbolize: the report has no traceback:\n%s", report)
		return
	}
	want := report[i:]
	// the report without its traceback: what phase 1 alone leaves
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	cmd := testtmp.Guard(exec.Command(xo, "dev", "symbolize", bin))
	cmd.Stdin = strings.NewReader(report[:i])
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != want {
		t.Errorf("xo dev symbolize: %v\n%s\nwant:\n%s", err, out, want)
	}
}

// TestCrashFileFault: XO_CRASH_FILE (decision 0520) also takes the
// report of a fault nothing caught (fault.xo exit): the "fault:" line
// standard error gets, and with XO_TRACEBACK=1 its frames. Unset, no
// file is made.
func TestCrashFileFault(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	res, err := xotool.Build([]string{filepath.Join("..", "llvm", "testdata", "golden", "abs_fault")}, xotool.Options{Backend: "arm64"})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	dir := t.TempDir()
	run := func(env ...string) string {
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Env = append(os.Environ(), env...)
		var errb strings.Builder
		cmd.Stderr = &errb
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 70 {
			t.Fatalf("%v: want exit 70, got %v\n%s", env, err, errb.String())
		}
		return errb.String()
	}
	cf := filepath.Join(dir, "fault.txt")
	got := run("XO_CRASH_FILE="+cf, "XO_TRACEBACK=1")
	b, err := os.ReadFile(cf)
	if err != nil || string(b) != got || !strings.HasPrefix(got, "fault: integer overflow: abs(") || !strings.Contains(got, "main (main.xo:") {
		t.Errorf("XO_CRASH_FILE: %v; the file:\n%s\nstderr:\n%s", err, b, got)
	}
	_ = run()
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Errorf("unset XO_CRASH_FILE made a file: %v", ents)
	}
}
