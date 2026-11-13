package selfhost

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	// the wasm run's counts come from the same module on arm64 (owner's
	// lead, 2026-11-13: hard-coded counts failed twice in a day on new
	// tests, not bugs): every test arm64 runs passes under wasm, less the
	// listed wasm skips, which name their reason; all marks a module
	// wasm skips whole
	for _, c := range []struct {
		mod    string
		skip   int
		all    bool
		reason string
	}{
		{mod: "base32"},
		{mod: "native/time"},
		{mod: "uuid"},
		{mod: "native/prelude"},
		{mod: "amqp", all: true, reason: "WASI preview 1 has no sockets"},
		{mod: "native/llrt/raw", skip: 1, reason: "a native Str view"},
	} {
		t.Run(strings.ReplaceAll(c.mod, "/", "_"), func(t *testing.T) {
			run := func(backend string, env ...string) string {
				cmd := testtmp.Guard(exec.Command(xo, "test", "--backend="+backend, "--count=1", "--std-module="+c.mod))
				cmd.Dir = root
				cmd.Env = append(append(os.Environ(), "XO_STD="+filepath.Join(root, "std")), env...)
				b, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("xo test --backend=%s --std-module=%s: %v\n%s", backend, c.mod, err, b)
				}
				return string(b)
			}
			native := run("arm64")
			m := wasmCountRE.FindStringSubmatch(native)
			if m == nil {
				t.Fatalf("xo test --backend=arm64 --std-module=%s: no count line\n%s", c.mod, native)
			}
			passed, _ := strconv.Atoi(m[1])
			nskip, _ := strconv.Atoi(m[2])
			total := passed + nskip
			if total == 0 {
				t.Fatalf("--std-module=%s ran no tests on arm64\n%s", c.mod, native)
			}
			var want string
			switch {
			case c.all:
				want = fmt.Sprintf("skipped (%d wasm: %s)", total, c.reason)
			case c.skip > 0:
				want = fmt.Sprintf("ok: %d passed, 0 failed, %d skipped (%d wasm: %s", total-c.skip, c.skip, c.skip, c.reason)
			default:
				want = fmt.Sprintf("ok: %d passed, 0 failed, 0 skipped", total)
			}
			got := run("wasm", "XO_TEST_EXEC="+node+" --no-warnings "+runner)
			if !strings.Contains(got, want) {
				t.Errorf("xo test --backend=wasm --std-module=%s (arm64 ran %d):\n%s\nwant a line with %q", c.mod, total, got, want)
			}
		})
	}
}

// wasmCountRE reads `xo test`'s summary line: passed and skipped.
var wasmCountRE = regexp.MustCompile(`ok: (\d+) passed, 0 failed, (\d+) skipped`)
