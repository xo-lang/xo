package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestInlineFallbackLoud: when the inlined arm64 module does not verify
// (here broken on purpose with XO_INLINE_BREAK=1), a build says so on
// standard error, naming the function, and goes on without inlining; with
// XO_INLINE_STRICT=1 (which internal/xotool sets for every test) it fails
// (gotchas/arm64-inline-verify-fallback.md: the fallback was silent).
func TestInlineFallbackLoud(t *testing.T) {
	cli, err := xotool.Command()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) uses (stdio) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	brk := ""
	build := func(strict string) (string, error) {
		args := append(append([]string{}, cli[1:]...), "build", "--backend=arm64", "-o", filepath.Join(dir, "prog"), ".")
		cmd := testtmp.Guard(exec.Command(cli[0], args...))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "XO_INLINE_BREAK="+brk, "XO_INLINE_STRICT="+strict, "XO_BACKEND_CACHE_DIR="+filepath.Join(dir, "cache"))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	// 1: verified as it is inlined; late: the verification deferred
	// until after dead function removal (main.xo defer_inline_verify),
	// with the per-function cache on
	for _, brk = range []string{"1", "late"} {
		out, err := build("1")
		if err == nil || !strings.Contains(out, "does not verify") || !strings.Contains(out, "XO_INLINE_STRICT=1") {
			t.Fatalf("%s strict: want a failed build that says the inlined module does not verify, got %v:\n%s", brk, err, out)
		}
		out, err = build("0")
		if err != nil || !strings.Contains(out, "does not verify") || !strings.Contains(out, "built without inlining") {
			t.Fatalf("%s normal: want a build that says it went on without inlining, got %v:\n%s", brk, err, out)
		}
	}
}

// TestAssumptionCheck (decision 0342): a module in which a call's result
// type disagrees with the callee's (broken on purpose with
// XO_ASSUME_BREAK=1, as a stale cached caller would have it) fails the
// build with XO1510 before code generation, on arm64 and amd64; unbroken
// it builds, and XO_ASSUME_CHECK=off skips the check.
func TestAssumptionCheck(t *testing.T) {
	cli, err := xotool.Command()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn twice(n: Int) -> Int {\n  n * 2\n}\n\nfn main(os: Os) uses (stdio) {\n  os.stdio.println(\"${twice(21)}\")\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(backend string, env ...string) (string, error) {
		args := append(append([]string{}, cli[1:]...), "build", "--backend="+backend, "-o", filepath.Join(dir, "prog-"+backend), ".")
		cmd := testtmp.Guard(exec.Command(cli[0], args...))
		cmd.Dir = dir
		cmd.Env = append(append(os.Environ(), "XO_BACKEND_CACHE_DIR="+filepath.Join(dir, "cache")), env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for _, be := range []string{"arm64", "amd64"} {
		out, err := build(be, "XO_ASSUME_BREAK=1")
		if err == nil || !strings.Contains(out, "XO1510") || !strings.Contains(out, "result type does not match the callee") {
			t.Errorf("%s broken: want XO1510 naming the call, got %v:\n%s", be, err, out)
		}
		if out, err := build(be); err != nil || strings.Contains(out, "XO1510") {
			t.Errorf("%s: want a clean build, got %v:\n%s", be, err, out)
		}
	}
	// skipped: the damage goes on to the inliner's verification, which
	// XO_INLINE_STRICT=1 (internal/xotool) makes an error of its own
	if out, _ := build("arm64", "XO_ASSUME_BREAK=1", "XO_ASSUME_CHECK=off"); strings.Contains(out, "XO1510") {
		t.Errorf("XO_ASSUME_CHECK=off still checked:\n%s", out)
	}
}
