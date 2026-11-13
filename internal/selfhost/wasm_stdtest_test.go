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

// TestWasmStdTests runs std modules' tests with `xo test --backend=wasm`
// under Node's WASI (XO_TEST_EXEC, decision 0610): the harness's channel
// and the test world work in a WASI test module (base32's property test,
// native/time's virtual clock and tickers, uuid's clock.advance, the
// prelude's FakeEnv through look_path), and what WASI preview 1 cannot run
// is a listed skip with its reason, never a pass or a build failure
// (amqp's sockets, raw's native Str view).
func TestWasmStdTests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test modules")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: wasm test modules run under Node's WASI")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	for _, c := range []struct{ mod, want string }{
		{"base32", "ok: 9 passed, 0 failed, 0 skipped"},
		{"native/time", "ok: 11 passed, 0 failed, 0 skipped"},
		{"uuid", "ok: 4 passed, 0 failed, 0 skipped"},
		{"native/prelude", "ok: 60 passed, 0 failed, 0 skipped"},
		{"amqp", "skipped (28 wasm: WASI preview 1 has no sockets)"},
		{"native/llrt/raw", "ok: 6 passed, 0 failed, 1 skipped (1 wasm: a native Str view"},
	} {
		t.Run(strings.ReplaceAll(c.mod, "/", "_"), func(t *testing.T) {
			cmd := testtmp.Guard(exec.Command(xo, "test", "--backend=wasm", "--count=1", "--std-module="+c.mod))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_TEST_EXEC="+node+" --no-warnings "+runner)
			b, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(b), c.want) {
				t.Errorf("xo test --backend=wasm --std-module=%s: %v\n%s\nwant a line with %q", c.mod, err, b, c.want)
			}
		})
	}
}
