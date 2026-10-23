package gen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/xo-lang/xo/internal/testtmp"
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
// XO_COMPILE_PROPS=N builds N (seeds 1..N, or from
// XO_COMPILE_PROP_FIRST) in the full suite (default compilePropFull;
// bench/nightly.sh builds 500, decision 0111).
// XO_COMPILE_PROP_SEED=S builds only seed S, and XO_COMPILE_PROP_KEEP=dir
// keeps the programs there; XO_COMPILE_PROP_LLVM=1 also compares the
// LLVM backend's run on an arm64 host. The programs live in directories
// named by their contents (xotool.TestProgramDir: an unchanged program
// reuses its binaries across runs) and the builds go to temporary ones,
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
	if v := os.Getenv("XO_COMPILE_PROP_FIRST"); v != "" {
		k, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("XO_COMPILE_PROP_FIRST=%q", v)
		}
		first = k
	}
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
	// XO_COMPILE_PROP_LLVM=1 also builds each program with the LLVM
	// backend where the native one is another, and compares its run
	alsoLLVM := os.Getenv("XO_COMPILE_PROP_LLVM") == "1"
	for seed := first; seed < first+n; seed++ {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			src := progen.Program(int64(seed))
			files := map[string]string{"xo.mod": "module example.com/prop\n", "main.xo": src}
			// the program in a directory named by its contents, so an
			// unchanged program reuses its binaries across runs
			// (xotool.TestProgramDir); outputs and runs in out
			out := t.TempDir()
			dir, err := xotool.TestProgramDir("compileprop", files)
			if keep != "" || err != nil {
				dir = out
				if keep != "" {
					dir = filepath.Join(keep, fmt.Sprint(seed))
					os.MkdirAll(dir, 0o755)
				}
				for name, text := range files {
					os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
				}
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Out: filepath.Join(out, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			if native == "" {
				return
			}
			nres, err := xotool.Build([]string{dir}, xotool.Options{NoGC: true, Backend: native, Out: filepath.Join(out, "prog-native")})
			if nres != nil && nres.Failed && err == nil && onlyNoGC(nres.Diagnostics) {
				t.Logf("seed %d: not native (%s)", seed, firstMsg(nres.Diagnostics))
				return
			}
			if err != nil || nres.Failed {
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s%s", seed, native, err, buildText(nres), numbered(src), triage("compile", int64(seed), files, native, true))
			}
			// both run alike: the same exit status, output, and
			// standard error (runProp)
			gr, nr := runProp(t, res.Binaries[xotool.Host()], out), runProp(t, nres.Binaries[xotool.Host()], out)
			if gr != nr {
				t.Errorf("seed %d: the native build runs differently\n--- go\n%s\n--- %s\n%s\n--- program\n%s%s", seed, gr, native, nr, numbered(src), triage("compile", int64(seed), files, native, false))
			}
			if alsoLLVM && native != xotool.BackendLLVM {
				lres, err := xotool.Build([]string{dir}, xotool.Options{NoGC: true, Backend: xotool.BackendLLVM, Out: filepath.Join(out, "prog-llvm")})
				if err != nil || lres.Failed {
					t.Fatalf("seed %d: the llvm build failed: %v\n%s\n--- program\n%s%s", seed, err, buildText(lres), numbered(src), triage("compile", int64(seed), files, xotool.BackendLLVM, true))
				}
				if lr := runProp(t, lres.Binaries[xotool.Host()], out); gr != lr {
					t.Errorf("seed %d: the llvm build runs differently\n--- go\n%s\n--- llvm\n%s\n--- program\n%s%s", seed, gr, lr, numbered(src), triage("compile", int64(seed), files, xotool.BackendLLVM, false))
				}
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
	cmd := testtmp.Guard(exec.CommandContext(ctx, bin))
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
//   - the frames under a `fault:` or `error:` line (indented lines, and
//     the native backends' `traceback:` header before them): the Go
//     backend prints Go's call stack (mangled function names, then
//     file:line), native backends their own;
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
		case frames && l == "traceback:\n":
			// the native backends' traceback header (by default since
			// dea6cca39, 2026-10-22); its frames are indented
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
	// a native fault's traceback (by default since 2026-10-22): header and
	// frames dropped, as the Go backend's frames are (TestArithProperty
	// failed every faulting seed on the full tier)
	tb := "fault: integer overflow: 3 * 4 does not fit Int at main.xo:2:3\ntraceback:\n  f0 (main.xo:2) [inlined]\n  main (main.xo:9)\n"
	if a, b := normStderr(goOut), normStderr(tb); a != b {
		t.Errorf("the Go fault and the native one with its traceback differ after normStderr:\n%q\n%q", a, b)
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
