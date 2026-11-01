package selfhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmComponentHTTP builds testdata/wasm_http, a program that calls
// http.serve_component, with `xo build --targets=wasip2/wasm`: a wasi:http 0.2
// component (decision 0447, compiler/wasm rt5.xo, rt6.xo, component.xo).
// testdata/wasm_http/host.mjs runs its core module under Node with stub
// imports written from the canonical ABI, so a wrong import signature
// fails instantiation and a wrong layout shows in the answers: with a new
// instance per request (wasmtime serve's default) and with one instance
// for every request, the output must be want.txt's.
func TestWasmComponentHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the component's core module runs under Node")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	dir := filepath.Join(root, "internal", "selfhost", "testdata", "wasm_http")
	out := filepath.Join(t.TempDir(), "wasm_http.wasm")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", out, "."))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build --targets=wasip2/wasm: %v\n%s", err, b)
	}
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	reqs := []string{"/", "/users/7?x=y", "/users/q", "/h", "POST /echo", "/json", "/nope"}
	var got []byte
	for _, mode := range []string{"fresh", "reuse"} {
		args := append([]string{filepath.Join(dir, "host.mjs"), out, mode}, reqs...)
		b, err := testtmp.Guard(exec.Command(node, args...)).CombinedOutput()
		if err != nil {
			t.Fatalf("host.mjs %s: %v\n%s", mode, err, b)
		}
		got = append(got, b...)
	}
	if string(got) != string(want) {
		t.Fatalf("the component's answers differ from want.txt\n--- got\n%s--- want\n%s", got, want)
	}
}
