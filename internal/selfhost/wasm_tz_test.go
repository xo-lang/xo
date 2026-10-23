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

// TestWasmTargetOS builds testdata/wasm_target with `xo build
// --backend=wasm` and runs it under Node's WASI: the program's `target`
// is Wasi on Wasm32, not the host's (field report b05, 2026-10-23). The
// build leaves no .wat beside the .wasm unless XO_KEEP_WAT=1.
func TestWasmTargetOS(t *testing.T) {
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
	dir := filepath.Join(root, "internal", "selfhost", "testdata", "wasm_target")
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	for _, keep := range []bool{false, true} {
		out := filepath.Join(t.TempDir(), "wasm_target.wasm")
		cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=wasm", "-o", out, "."))
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off", "XO_KEEP_WAT=")
		if keep {
			cmd.Env = append(cmd.Env, "XO_KEEP_WAT=1")
		}
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=wasm: %v\n%s", err, b)
		}
		_, werr := os.Stat(strings.TrimSuffix(out, ".wasm") + ".wat")
		if keep && werr != nil {
			t.Errorf("XO_KEEP_WAT=1: no .wat written: %v", werr)
		}
		if !keep && werr == nil {
			t.Errorf("a .wat was written without XO_KEEP_WAT=1")
		}
		if got := runWasm(t, node, runner, out, dir, false); got.String() != string(want) {
			t.Fatalf("the module's run differs from want.txt\n--- got\n%s--- want\n%s", got.String(), want)
		}
	}
}
