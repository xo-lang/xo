package arm64

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestJoinFactsBudget: the bounds proof's facts past a short-circuit
// join (compiler/arm64 fieldlicm.xo join_facts, task #457) cost nothing
// per fact. Its first form copied every fact of the block the join's
// value came from to the taken side, so a function's joins times its
// facts were walked, each with a walk up the dominator tree: xo's own
// build did not end in twenty minutes. One function of 1500 `a && b`
// conditions compiles in 5.4 s on the Mac Studio, as with main's
// compiler (5.35 s), and took 93 s with the copying form: the 45 s
// budget is far from both.
func TestJoinFactsBudget(t *testing.T) {
	t.Parallel()
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	var b strings.Builder
	b.WriteString("fn work(xs: List[Int], a: Int, b: Int, c: Int, d: Int) -> Int {\n  var n = 0\n")
	for i := 0; i < 1500; i++ {
		fmt.Fprintf(&b, "  if a + %d < b && c + %d < d {\n    n += xs[n %% xs.len()]\n  }\n", i, i)
	}
	b.WriteString("  n\n}\n\nfn main(os: Os) {\n  let k = os.env.args().len()\n  os.stdio.println(\"${work([1, 2, 3], k, k + 700, k, k + 900)}\")\n}\n")
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	args := append(append([]string{}, cli[1:]...), "asm", "--backend=arm64", "--target=darwin/arm64", dir)
	cmd := testtmp.Guard(exec.CommandContext(ctx, cli[0], args...))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	start := time.Now()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo asm of 1500 short-circuit conditions: %v after %s (budget 45 s)\n%s", err, time.Since(start).Round(time.Second), stderr.String())
	}
	if !strings.Contains(string(out), "work") {
		t.Fatalf("no code for work in the output (%d bytes)", len(out))
	}
	t.Logf("compiled in %s", time.Since(start).Round(10*time.Millisecond))
}
