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

// playStackMB is the worker stack the playground test gives the
// playground's compiler: about what a dedicated Worker of Chrome allows
// (0.63 of a 1 MB Node worker, gotchas/wasm-browser-stack-locals.md).
const playStackMB = "0.6"

// playMaxBytes bounds the playground compiler (compiler/wasm built for
// wasm): 26,180,380 bytes at 2026-11-11 (main after the fault zero fix),
// with about a fifth of room for the compiler's growth.
const playMaxBytes = 32_000_000

// playSource is the playground tour's first example (the website's
// examples/tour/basics), the program every visitor runs first.
const playSource = `const GREETING = "hello"

fn greet(name: Str) -> Str {
  "${GREETING}, ${name}"
}

fn sign(n: Int) -> Str {
  if n < 0 { "negative" } else if n == 0 { "zero" } else { "positive" }
}

fn main(os: Os) {
  let who = "Ada"
  var count = 0
  for i in 0..3 {
    count += i
  }
  os.stdio.println(greet(who))
  os.stdio.println("count = ${count}, sign = ${sign(count - 5)}")
}
`

// TestWasmPlaygroundStack builds the playground's compiler as the website
// does (`xo build --backend=wasm compiler/wasm`, scripts/build-playground.py
// in the website repo) and runs `--play build` with it in a Node worker
// with a browser worker's stack: writing std's declaration record, then
// loading it, as the page does. The other wasm tests run compiler/wasm
// built natively, so 0.48.0's compiler overflowed every browser's stack
// on every build with all of them passing (gotchas/
// wasm-playground-stack-overflow.md); only the website's own test, run
// when the playground is rebuilt, would have caught it.
func TestWasmPlaygroundStack(t *testing.T) {
	if testing.Short() || selfhostTier(t) == tierShort {
		t.Skip("builds compiler/wasm for wasm (merge and full tiers)")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the playground's compiler runs under Node's WASI")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	tmp := t.TempDir()
	xoc := filepath.Join(tmp, "xoc.wasm")
	cmd := testtmp.Guard(exec.Command(xo, "build", "--backend=wasm", "-o", xoc, "compiler/wasm"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build --backend=wasm compiler/wasm: %v\n%s", err, b)
	}
	// the playground's download: nothing else watched the wasm build's size,
	// and a fault exit change doubled it unseen (22.5 to 44.8 MB, 0ce3d8803,
	// gotchas/wasm-fault-zero-call-home-fallback.md)
	if fi, err := os.Stat(xoc); err != nil {
		t.Fatal(err)
	} else if fi.Size() > playMaxBytes {
		t.Fatalf("compiler/wasm built for wasm is %d bytes, over the %d bound (playMaxBytes): the playground downloads it; find what grew (bench/results/xoc-size-2026-11-11.md's method) before raising the bound", fi.Size(), playMaxBytes)
	}
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(playSource), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "xo.mod"), []byte("module example.com/play\nedition preview\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := filepath.Join(tmp, "std.rec")
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	for _, step := range []struct{ name, env string }{
		{"writing std's record", "XO_PLAY_RECORD_OUT=" + rec},
		{"with std's record", "XO_PLAY_RECORD=" + rec},
	} {
		c := testtmp.Guard(exec.Command(node, "--no-warnings", runner, xoc, "--std="+filepath.Join(root, "std"), "--wd="+tmp, "--play", "build", src, filepath.Join(tmp, "prog.wasm")))
		c.Dir = tmp
		c.Env = append(os.Environ(), "XO_WASM_STACK_MB="+playStackMB, step.env)
		b, err := c.CombinedOutput()
		out := strings.TrimSpace(string(b))
		if err != nil || !strings.Contains(out, `{"ok":true`) {
			t.Fatalf("--play build %s in a %s MB worker stack: %v\n%s", step.name, playStackMB, err, out)
		}
		if strings.HasPrefix(step.env, "XO_PLAY_RECORD_OUT=") {
			if fi, err := os.Stat(rec); err != nil || fi.Size() == 0 {
				t.Fatalf("--play build wrote no record: %v", err)
			}
		}
	}
}
