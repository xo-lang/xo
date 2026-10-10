package gen_test

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

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/progen"
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
	if _, err := build.Clang(); err == nil {
		native = build.NativeBackend()
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
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Out: filepath.Join(dir, "prog")})
			if err != nil || res.Failed {
				t.Fatalf("seed %d: the Go build failed: %v\n%s\n--- program\n%s", seed, err, diagText(res), numbered(src))
			}
			if native == "" {
				return
			}
			nres, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, NoGC: true, Backend: native, Out: filepath.Join(dir, "prog-native")})
			if nres != nil && nres.Failed && err == nil && onlyNoGC(nres.Diagnostics) {
				t.Logf("seed %d: not native (%s)", seed, firstMsg(nres.Diagnostics))
				return
			}
			if err != nil || nres.Failed {
				t.Fatalf("seed %d: the native build (%s) failed: %v\n%s\n--- program\n%s", seed, native, err, diagText(nres), numbered(src))
			}
			// both run alike: the same exit status, output, and first
			// line of standard error
			gr, nr := runProp(t, res.Binaries[build.Host()], dir), runProp(t, nres.Binaries[build.Host()], dir)
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

// runProp runs a generated program: its exit status, standard output,
// and the first line of standard error (frames and crash bundle lines
// differ by backend).
func runProp(t *testing.T, bin, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
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
	first, _, _ := strings.Cut(stderr.String(), "\n")
	if strings.HasPrefix(first, "fault: XO-F001") {
		// natively the deadlock fault has no location (decision 0083)
		first, _, _ = strings.Cut(first, " at ")
	}
	return fmt.Sprintf("exit %d\n%s--- stderr\n%s", code, stdout.String(), first)
}

func onlyNoGC(ds []diag.Diagnostic) bool {
	n := 0
	for _, d := range ds {
		if d.Severity != diag.Error {
			continue
		}
		if d.Code != string(diag.NoGCUnavailable) {
			return false
		}
		n++
	}
	return n > 0
}

func firstMsg(ds []diag.Diagnostic) string {
	for _, d := range ds {
		if d.Severity == diag.Error {
			return d.Message
		}
	}
	return ""
}

func diagText(res *build.Result) string {
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

// ---------------------------------------------------------------------------
// The generator
