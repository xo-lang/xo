package selfhost

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// wasmMapGoldens are the Map and Set goldens TestWasmMapHybrid runs.
var wasmMapGoldens = []string{"map_log_model", "map_log_sets", "maps", "map_small", "map_until", "native_map_eq", "set_hofs", "nested_update", "map_remove_bytes", "big_struct"}

// TestWasmMapHybrid runs the Map goldens on the wasm backend in both of
// decision 0291's forms: the hybrid (the default, which TestSelfhostWasm
// covers for every golden) and the lists alone (XO_MAP_LOG=0). The
// hybrid's checks once read the native runtime's object layout there,
// and a new map looked marked and a log value looked like lists
// (2026-10-21).
func TestWasmMapHybrid(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds compiler/wasm (merge and full tiers)")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the wasm goldens run under Node's WASI")
	}
	bin, err := selfhostBinary("wasm", "go")
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	setModEnv(t, root)
	for _, mode := range []string{"1", "0"} {
		t.Run("map_log="+mode, func(t *testing.T) { wasmMapRun(t, node, bin, root, mode) })
	}
}

func wasmMapRun(t *testing.T, node, bin, root, mode string) {
	out := t.TempDir()
	var rel []string
	for _, g := range wasmMapGoldens {
		rel = append(rel, filepath.Join("internal", "llvm", "testdata", "golden", g))
	}
	cmd := exec.Command(bin, append([]string{"--std=std", "--wd=" + root, "--each", "--out=" + out}, rel...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_MAP_LOG="+mode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	res, err := cmd.Output()
	if err != nil {
		t.Fatalf("compiler/wasm: %v\n%s", err, stderr.String())
	}
	results := parseWasmRuns(string(res))
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	for _, r := range rel {
		name := filepath.Base(r)
		line := results[r]
		if !strings.HasPrefix(line, "== ok ") {
			t.Errorf("%s: %q", name, line)
			continue
		}
		dir := filepath.Join(root, r)
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		wasm := filepath.Join(out, name+".wasm")
		got := runWasm(t, node, runner, wasm, dir, false)
		if got.String() != string(want) {
			t.Errorf("%s: the hybrid Map's wasm run differs from want.txt\n--- got\n%s--- want\n%s", name, got.String(), want)
			continue
		}
		if got.exit == 0 {
			lk := runWasm(t, node, runner, wasm, dir, true)
			if m := wasmLeakRE.FindStringSubmatch(lk.rawStderr); m == nil || m[1] != "0" {
				t.Errorf("%s: leak check: %q", name, lk.rawStderr)
			}
		}
	}
}
