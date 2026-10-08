package rt

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// MixHash combines a hash with another.
func MixHash(h, x uint64) uint64 { return mix(h, x) }

// DebugTo writes the Debug text of v.
func DebugTo(b *strings.Builder, v any) { debugTo(b, v) }

// FMod is `a % b` on floats.
func FMod[F ~float32 | ~float64](a, b F) F { return F(math.Mod(float64(a), float64(b))) }

// FDiv is `a / b` on floats (IEEE 754: division by zero is infinite or
// NaN, not a fault).
func FDiv[F ~float32 | ~float64](a, b F) F { return a / b }

// IntMulDur is checked `n * d`.
func IntMulDur(n int64, d Duration, at string) Duration { return MulS(Duration(n), d, at) }

// CurrentTarget is the predeclared `target` (core 12.2): the target the
// program was built for.
func CurrentTarget() Target {
	t := Target{F_os: TargetOs{Tag: 1}, F_arch: TargetArch{Tag: 1}}
	switch runtime.GOOS {
	case "darwin":
		t.F_os = TargetOs{Tag: 2}
	case "windows":
		t.F_os = TargetOs{Tag: 3}
	case "wasip1", "wasi":
		t.F_os = TargetOs{Tag: 4}
	}
	switch runtime.GOARCH {
	case "arm64":
		t.F_arch = TargetArch{Tag: 2}
	case "wasm":
		t.F_arch = TargetArch{Tag: 3}
	}
	return t
}

func stderrWrite(s string) { outWrite(os.Stderr, s) }

func stderrFile() *os.File { return os.Stderr }

// exitUnwind unwinds a test's code under test after `os.proc.exit(code)`
// (std.md 8.4).
type exitUnwind struct{ code int64 }

// RunMain runs a program's `main` (core 6.3, 7.5, 8, 11.2) and returns the
// process exit status: 0, 1 for an error returned from main, 70 for a
// fault. main runs as the body of the root scope, whose group is
// `os.tasks`. The first SIGINT or SIGTERM cancels the root scope; a second
// exits with status 130.
func RunMain(main func(*Task, Os) Result[Unit, IError]) (code int) {
	root := NewRootTask()
	s := NewScope(root)
	osv := RealOs(s.M_group(root))
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		s.bodyNode.cancel()
		<-sig
		os.Exit(130)
	}()
	defer signal.Stop(sig)
	var res Result[Unit, IError]
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if IsUnwind(r) {
			code = 0
			return
		}
		f := AsFault(r)
		reportFault(f)
		code = 70
	}()
	ctl := func() (c int) {
		tk := s.Body()
		completed := false
		defer func() { c = s.Close(recover(), completed, c) }()
		res = main(tk, osv)
		completed = true
		return 0
	}()
	if ctl == CtlScopeFail {
		e, tr := s.Failure()
		printError(root, e, tr)
		return 1
	}
	if !res.Ok {
		printError(root, res.E, res.Tr)
		return 1
	}
	return 0
}

// printError writes an error from main with its frames (core 11.1).
func printError(tk *Task, e any, tr *Trace) {
	var b strings.Builder
	b.WriteString("error: ")
	b.WriteString(Display(tk, e))
	b.WriteByte('\n')
	for _, l := range tr.Lines() {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	stderrWrite(b.String())
}

// reportFault writes a fault that reached main and its crash bundle.
func reportFault(f *FaultErr) {
	var b strings.Builder
	b.WriteString("fault: " + f.Error() + "\n")
	b.WriteString(f.Stack)
	if p := writeCrashBundle(f); p != "" {
		b.WriteString("crash bundle: " + p + "\n")
	}
	stderrWrite(b.String())
}

// writeCrashBundle writes `<name>-<time>.xocrash` (core 11.2) into
// $XO_CRASH_DIR, or the system temporary directory.
func writeCrashBundle(f *FaultErr) string {
	dir := os.Getenv("XO_CRASH_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	name := filepath.Base(os.Args[0])
	now := time.Now().UTC()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.xocrash", name, now.Format("20060102T150405.000000000")))
	bundle := map[string]any{
		"fault":    f.Msg,
		"code":     f.Code,
		"location": f.Loc,
		"stack":    strings.Split(strings.TrimRight(f.Stack, "\n"), "\n"),
		"target":   runtime.GOOS + "/" + runtime.GOARCH,
		"time":     now.Format(time.RFC3339Nano),
		"args":     os.Args,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return ""
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ""
	}
	return path
}
