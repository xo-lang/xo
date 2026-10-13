package gen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/progen"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestCompileProperty is a property test of the toolchain (decision 0110):
// every program the checker accepts must build. It generates programs
// that use the optional parts of the syntax in random combinations
// (select arms with and without a binding, with send and after; closures
// with and without declared parameter, result, and error types; match
// arms with every kind of pattern, or patterns that bind, and guards;
// defer, scopes, spawn, `?`, `??`, optional chaining, interpolation, Map
// and List literals, struct `with`, `while` with break and labels). Each
// must check clean (a diagnostic is a bug of the generator), build on the
// Go backend, and, when the native check accepts it (no XO1201), build
// with `--no-gc` too; both builds then run, and must exit alike and print
// the same output.
//
// -short builds compilePropShort programs from fixed seeds;
// XO_COMPILE_PROPS=N builds N (seeds 1..N) in the full suite (default
// compilePropFull; bench/nightly.sh builds 500, decision 0111).
// XO_COMPILE_PROP_SEED=S builds only seed S, and XO_COMPILE_PROP_KEEP=dir
// keeps the programs there. The programs live in temporary directories,
// so the test runs beside the other top level tests.
func TestCompileProperty(t *testing.T) {
	t.Parallel()
	n := compilePropFull
	if testing.Short() {
		n = compilePropShort
	}
	if v := os.Getenv("XO_COMPILE_PROPS"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil || k < 1 {
			t.Fatalf("XO_COMPILE_PROPS=%q: want a positive count", v)
		}
		n = k
	}
	first := 1
	if v := os.Getenv("XO_COMPILE_PROP_SEED"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("XO_COMPILE_PROP_SEED=%q", v)
		}
		first, n = k, 1
	}
	native := ""
	if _, err := xotool.Clang(); err == nil {
		native = xotool.NativeBackend()
	}
	keep := os.Getenv("XO_COMPILE_PROP_KEEP")
	for seed := first; seed < first+n; seed++ {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			src := progen.Program(int64(seed))
			dir := t.TempDir()
			if keep != "" {
				dir = filepath.Join(keep, fmt.Sprint(seed))
				os.MkdirAll(dir, 0o755)
			}
			os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/prop\n"), 0o644)
			os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644)
			res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(dir, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			if native == "" {
				return
			}
			nres, err := xotool.Build([]string{dir}, xotool.Options{NoGC: true, Backend: native, Out: filepath.Join(dir, "prog-native")})
			if nres != nil && nres.Failed && err == nil && onlyNoGC(nres.Diagnostics) {
				t.Logf("seed %d: not native (%s)", seed, firstMsg(nres.Diagnostics))
				return
			}
			if err != nil || nres.Failed {
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s", seed, native, err, diagText(nres), numbered(src))
			}
			// both run alike: the same exit status, output, and
			// standard error (runProp)
			gr, nr := runProp(t, res.Binaries[xotool.Host()], dir), runProp(t, nres.Binaries[xotool.Host()], dir)
			if gr != nr {
				t.Errorf("seed %d: the native build runs differently\n--- go\n%s\n--- %s\n%s\n--- program\n%s", seed, gr, native, nr, numbered(src))
			}
		})
	}
}

const (
	compilePropShort = 24
	compilePropFull  = 50
)

// runProp runs a generated program and returns what every backend must
// agree on: its exit status, all of its standard output, and all of its
// standard error after normStderr.
func runProp(t *testing.T, bin, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	// crash bundles go to a directory the test removes, not $TMPDIR
	cmd.Env = append(os.Environ(), "XO_CRASH_DIR="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %s: %v", bin, err)
		}
		code = ee.ExitCode()
	}
	return fmt.Sprintf("exit %d\n%s--- stderr\n%s", code, stdout.String(), normStderr(stderr.String()))
}

// normStderr removes from a program's standard error only what
// legitimately differs between backends, and keeps every other line:
//
//   - the frames under a `fault:` or `error:` line (indented lines): the
//     Go backend prints Go's call stack (mangled function names, then
//     file:line), native backends print none;
//   - `crash bundle: PATH` lines: the path holds a time stamp, and only
//     the Go backend writes a bundle;
//   - the location of the deadlock fault XO-F001 and its list of blocked
//     tasks (`blocked:` and every line after it but a crash bundle line):
//     natively there are none (decision 0083).
func normStderr(s string) string {
	var b strings.Builder
	frames, deadlock, blocked := false, false, false
	for _, l := range strings.SplitAfter(s, "\n") {
		if l == "" {
			continue
		}
		if deadlock && l == "blocked:\n" {
			blocked = true
		}
		switch {
		case blocked && !strings.HasPrefix(l, "crash bundle: "):
			continue
		case frames && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")):
			continue
		case strings.HasPrefix(l, "crash bundle: "):
			continue
		}
		frames = strings.HasPrefix(l, "fault: ") || strings.HasPrefix(l, "error: ")
		deadlock = strings.HasPrefix(l, "fault: XO-F001")
		if deadlock {
			if i := strings.Index(l, " at "); i >= 0 {
				l = l[:i] + "\n"
			}
		}
		b.WriteString(l)
	}
	return b.String()
}

func onlyNoGC(ds []xotool.Diagnostic) bool {
	n := 0
	for _, d := range ds {
		if d.Severity != "error" {
			continue
		}
		if d.Code != "XO1201" {
			return false
		}
		n++
	}
	return n > 0
}

func firstMsg(ds []xotool.Diagnostic) string {
	for _, d := range ds {
		if d.Severity == "error" {
			return d.Message
		}
	}
	return ""
}

func diagText(res *xotool.Result) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, d := range res.Diagnostics {
		fmt.Fprintf(&b, "%d:%d: %s %s: %s\n", d.Span.Line, d.Span.Col, d.Severity, d.Code, d.Message)
	}
	return b.String()
}

func numbered(src string) string {
	var b strings.Builder
	for i, l := range strings.Split(src, "\n") {
		fmt.Fprintf(&b, "%4d  %s\n", i+1, l)
	}
	return b.String()
}

func TestNormStderr(t *testing.T) {
	goOut := "fault: integer overflow: 3 * 4 does not fit Int at main.xo:2:3\n  x_example_com_r.F_f\n      main.xo:2\ncrash bundle: /var/x/r-20261011T044433.xocrash\n"
	native := "fault: integer overflow: 3 * 4 does not fit Int at main.xo:2:3\n"
	if a, b := normStderr(goOut), normStderr(native); a != b {
		t.Errorf("the Go and native faults differ after normStderr:\n%q\n%q", a, b)
	}
	if got := normStderr("fault: XO-F001: deadlock at main.xo:3:1\nblocked:\nmain blocked in select at main.xo:52\n\ncrash bundle: x\n"); got != "fault: XO-F001: deadlock\n" {
		t.Errorf("deadlock: %q", got)
	}
	// indented lines that are not frames, and later lines, are kept
	if got, want := normStderr("  note\nother\n"), "  note\nother\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// The generator
