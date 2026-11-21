package llvm

import (
	"bytes"
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

// giantChainStmts is the statements of TestGiantFnCompileTime's main:
// about 6.4 basic blocks each, so past split.xo's GIANT_FN_BLOCKS (8000).
const giantChainStmts = 2000

// giantChainProg is a main of n statements each of which uses the last
// one's value, as pass_const_edges' main is: checked arithmetic on h, a
// call of a small helper, and a debug value record of h per statement.
func giantChainProg(n int) string {
	var b strings.Builder
	b.WriteString("fn op(a: Int, b: Int) -> Int { a.wrapping_add(b) }\n\n")
	b.WriteString("fn main(os: Os) {\n  let k = os.env.args().len()\n  var h = 0\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  h = h * 31 %% 1000000007 + (op(k, %d) %% 1000000007).abs()\n", i)
	}
	b.WriteString("  os.stdio.println(\"${k} ${h}\")\n}\n")
	return b.String()
}

// TestGiantFnCompileTime guards the LLVM build time of one very large
// function (task #586): pass_const_edges' main, 1,521 such statements,
// took clang 93 s on the Mac and 235 s on linux/arm64, nearly all of it
// in InstCombine moving debug value records as it sank instructions
// along the chain (then EarlyCSE and the inliner over what it left);
// every optimized module is compiled with code sinking off (compiler/xo
// split.xo giant_fn_flags) and this one takes 4 s. The budget is the
// build's user CPU time (the load of the host does not move it): 2,000
// statements took 4.9 s with the option and 102.5 s without it on the
// Mac (2026-11-21).
func TestGiantFnCompileTime(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a 2,000 statement function with clang")
	}
	if _, err := xotool.Clang(); err != nil {
		t.Skip(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module giantchain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(giantChainProg(giantChainStmts)), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "prog")
	// every cache off: a cached object would be no clang run at all, the
	// test binary's clang cache too (xotool.EnableTestClangCache keeps
	// the real clang in XO_CLANG_CACHED)
	cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=llvm", "-o", bin, "."))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_BIN_CACHE=off", "XO_BACKEND_CACHE=off", "XO_BUILD_DIR="+filepath.Join(dir, "build"))
	if real := os.Getenv("XO_CLANG_CACHED"); real != "" {
		cmd.Env = append(cmd.Env, "XO_CLANG="+real)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	// the build's children (clang, the linker) are in its user time
	user := cmd.ProcessState.UserTime()
	t.Logf("%d statements: %v of user CPU time", giantChainStmts, user)
	const budget = 75 * time.Second
	if user > budget {
		t.Errorf("the build took %v of user CPU time, over %v: a giant function's module is compiled with InstCombine's code sinking off (split.xo giant_fn_flags)", user, budget)
	}
	run := testtmp.Guard(exec.Command(bin))
	var stdout bytes.Buffer
	run.Stdout = &stdout
	if err := run.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	// k is the program's argument count as it sees it
	var k, got int64
	if _, err := fmt.Sscan(stdout.String(), &k, &got); err != nil {
		t.Fatalf("output %q: %v", stdout.String(), err)
	}
	var h int64
	for i := int64(0); i < giantChainStmts; i++ {
		v := (k + i) % 1000000007
		if v < 0 {
			v = -v
		}
		h = h*31%1000000007 + v
	}
	if got != h {
		t.Errorf("h = %d, want %d (k %d)", got, h, k)
	}
}
