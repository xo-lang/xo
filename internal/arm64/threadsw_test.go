package arm64

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestThreadedDispatch: interp's run (a 10 arm match in a loop, a jump
// table) ends each arm in its own table branch (decision 0397,
// compiler/arm64 threadsw.xo), and is one `br` with the pass off.
func TestThreadedDispatch(t *testing.T) {
	dir := filepath.Join(benchDir, "interp")
	count := func(env []string) int {
		out, err := xotool.AsmEnv(dir, xotool.BackendArm64, "darwin/arm64", env)
		if err != nil {
			t.Fatal(err)
		}
		run := funcBlock(string(out), "run")
		if run == "" {
			t.Fatalf("no `// func run` block in the assembly")
		}
		return strings.Count(run, "\tbr x16\n")
	}
	if n := count(nil); n < 9 {
		t.Errorf("threaded run: %d table branches, want one per arm (at least 9)", n)
	}
	if n := count([]string{"XO_PASSES=-thread_switch"}); n != 1 {
		t.Errorf("run with thread_switch off: %d table branches, want 1", n)
	}
}

// funcBlock is the `// func NAME` block of an assembly text, up to the
// blank line after it (ownFuncs's shape), or "".
func funcBlock(text, name string) string {
	var b strings.Builder
	keep := false
	for _, l := range strings.SplitAfter(text, "\n") {
		if n, ok := strings.CutPrefix(l, "// func "); ok {
			keep = strings.TrimSpace(n) == name
		}
		if keep {
			b.WriteString(l)
			if l == "\n" {
				break
			}
		}
	}
	return b.String()
}
