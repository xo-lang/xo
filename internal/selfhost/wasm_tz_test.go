package selfhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmZoneData builds internal/llvm's tz_table golden with `xo build
// --backend=wasm`, which embeds the TZif data of the program's constant
// zone names in the module (compiler/tzsel, compiler/wasm rt4.xo
// tz_table_fn), and runs it under Node's WASI: its output must be
// want.txt's and it must free every allocation. compiler/wasm's own runs
// (TestSelfhostWasm, the playground) embed no zone data, so there the
// golden is unsupported.
func TestWasmZoneData(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: wasm programs run under Node's WASI")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "llvm", "testdata", "golden", "tz_table")
	out := filepath.Join(t.TempDir(), "tz_table.wasm")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=wasm", "-o", out, "."))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=wasm: %v\n%s", err, b)
	}
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	if got := runWasm(t, node, runner, out, dir, false); got.String() != string(want) {
		t.Fatalf("the module's run differs from want.txt\n--- got\n%s--- want\n%s", got.String(), want)
	}
	lk := runWasm(t, node, runner, out, dir, true)
	if m := wasmLeakRE.FindStringSubmatch(lk.rawStderr); m == nil || m[1] != "0" {
		t.Errorf("leak check: %q", lk.rawStderr)
	}
}
