package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		cmd := exec.Command(cli[0], args...)
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
