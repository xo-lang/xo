package selfhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmReactorLib builds testdata/wasm_reactor with `xo build
// --lib=shared --targets=wasip1/wasm`, a WASI reactor whose exports are
// its `as c` functions (decision 0688), and drives it from Node
// (drive.mjs): Int and Float arguments and results, Str and Bytes
// arguments through xo_alloc, Str results through xo_result_len, error
// results through xo_failed and xo_last_error, a fault that traps, and no
// object left live after a thousand calls of each kind. Its stdout must be
// want.txt's. A library with no `as c` function is refused.
func TestWasmReactorLib(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a library")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the reactor runs under Node's WASI")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "selfhost", "testdata", "wasm_reactor")
	out := filepath.Join(t.TempDir(), "lib.wasm")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--lib=shared", "--targets=wasip1/wasm", "-o", out, "."))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build --lib=shared --targets=wasip1/wasm: %v\n%s", err, b)
	}
	run := testtmp.Guard(exec.Command(node, "--no-warnings", filepath.Join(dir, "drive.mjs"), out))
	var stdout, stderr []byte
	stdout, err = run.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		stderr = ee.Stderr
	}
	if err != nil {
		t.Fatalf("node drive.mjs: %v\n%s%s", err, stdout, stderr)
	}
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stdout) != string(want) {
		t.Fatalf("drive.mjs's output differs from want.txt\n--- got\n%s--- want\n%s", stdout, want)
	}

	// a library with no `as c` function exports nothing: an error
	empty := testtmp.Guard(exec.Command(xo, "build", "--lib=shared", "--targets=wasip1/wasm", "-o", filepath.Join(t.TempDir(), "empty.wasm"), "."))
	empty.Dir = filepath.Join(root, "internal", "selfhost", "testdata", "wasm_reactor_empty")
	empty.Env = cmd.Env
	b, err := empty.CombinedOutput()
	if err == nil || !strings.Contains(string(b), "no `as c` function") {
		t.Errorf("a library with no as c function: err %v, output %s", err, b)
	}
}
