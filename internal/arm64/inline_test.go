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
	build := func(strict string) (string, error) {
		args := append(append([]string{}, cli[1:]...), "build", "--backend=arm64", "-o", filepath.Join(dir, "prog"), ".")
		cmd := exec.Command(cli[0], args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "XO_INLINE_BREAK=1", "XO_INLINE_STRICT="+strict)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := build("1")
	if err == nil || !strings.Contains(out, "does not verify") || !strings.Contains(out, "XO_INLINE_STRICT=1") {
		t.Fatalf("strict: want a failed build that says the inlined module does not verify, got %v:\n%s", err, out)
	}
	out, err = build("0")
	if err != nil || !strings.Contains(out, "does not verify") || !strings.Contains(out, "built without inlining") {
		t.Fatalf("normal: want a build that says it went on without inlining, got %v:\n%s", err, out)
	}
}
