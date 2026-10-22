package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"

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
	// the C function (LLVM names it with its module's prefix), then poke
	// at its call (the match's line or the arm's), the recursion, main
	frames := regexp.MustCompile(`(?m)^  \S*crashc_write\n  poke \(main\.xo:1[4-7]\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n  main \(main\.xo:32\)$`)
	if !frames.MatchString(got) {
		t.Errorf("segv: the traceback lacks the frames:\n%s", got)
	}
	if strings.Contains(got, "crash report faulted") {
		t.Errorf("segv: the handler ran twice:\n%s", got)
	}
	if !strings.HasPrefix(got, "fatal error: SIGSEGV") {
		t.Errorf("segv: the report does not start with its first line:\n%s", got)
	}
	// xo dev symbolize (stage 3): the same traceback from the report's
	// frames and the binary alone
	symbolize(t, bin, got)
	// the trap instruction: brk on arm64 (SIGTRAP)
	_, got, sig = run("XO_CRASH_KIND=trap")
	if sig != syscall.SIGTRAP || !strings.HasPrefix(got, "fatal error: SIGTRAP: trace trap (code ") || strings.Contains(got, "crash report faulted") || !regexp.MustCompile(`crashc_trap\n  poke \(main\.xo:1[45]\)\n`).MatchString(got) {
		t.Errorf("trap: signal %v\n%s", sig, got)
	}
	// a stack overflow in C: reported on the alternate signal stack
	_, got, sig = run("XO_CRASH_KIND=overflow")
	if (sig != syscall.SIGSEGV && sig != syscall.SIGBUS) || !strings.HasPrefix(got, "fatal error: SIG") || !strings.Contains(got, "crashc_deep") {
		t.Errorf("overflow: signal %v\n%s", sig, got)
	}
	// a fault in a task (a worker's fiber): its own frames
	_, got, sig = run("XO_CRASH_KIND=task")
	if sig != syscall.SIGSEGV || !strings.HasPrefix(got, "fatal error: SIGSEGV") || !regexp.MustCompile(`crashc_write\n  poke \(main\.xo:1[4-7]\)\n  poke \(main\.xo:10\)\n  poke \(main\.xo:10\)\n`).MatchString(got) {
		t.Errorf("task: signal %v\n%s", sig, got)
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
