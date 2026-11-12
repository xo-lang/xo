package selfhost

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

// TestPlayReplType: the playground's REPL (--play repl, compiler/wasm
// playrepl.xo) answers `:type` and records an entry's type. Its query
// session named the session's files from the session directory, but the
// check names them from the working directory (sess/main.xo), so every
// `:type` said "main.xo is not part of the checked modules" and no value
// printed its type (2026-11-11). The REPL's code is the same on every
// backend: this builds compiler/wasm natively, which takes seconds.
func TestPlayReplType(t *testing.T) {
	// a binding's entry is built and its run answered (empty output), so
	// the next requests see it
	in := strings.Join([]string{
		`{"cmd": "type", "arg": "3.5 * 2.0"}`,
		`{"in": "let xs = [1, 2]"}`,
		`{"ran": "", "code": 0}`,
		`{"cmd": "type", "arg": "xs.map(fn(x) { x > 1 })"}`,
		// a var binding has its type too (so it is pinned, and a later
		// `ys.push(2)` entry's change is kept: TestReplVarMethodCall)
		`{"in": "var ys = [1]"}`,
		`{"ran": "", "code": 0}`,
	}, "\n") + "\n"
	out := playRepl(t, in)
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{`"type":"Float"`, `"run":`, `"type":"List[Int]"`, `"type":"List[Bool]"`, `"run":`, `"type":"List[Int]"`}
	if len(got) != len(want) {
		t.Fatalf("--play repl: %d replies, want %d:\n%s", len(got), len(want), out)
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) || (w != `"run":` && !strings.Contains(got[i], `"ok":true`)) {
			t.Errorf("reply %d: %s, want %s and ok", i+1, got[i], w)
		}
	}
}

// playRepl builds compiler/wasm natively and runs `--play repl` over
// the request lines in, with the playground's warm program; it returns
// the replies.
func playRepl(t *testing.T, in string) []byte {
	t.Helper()
	tmp := t.TempDir()
	xoc, root := playCompiler(t, tmp)
	for d, f := range map[string]string{
		"warm/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"${[1, 2, 3].len()}\")\n}\n",
		"warm/xo.mod":  "module warm\n",
		"sess/xo.mod":  "module sess\n",
	} {
		p := filepath.Join(tmp, d)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := testtmp.Guard(exec.Command(xoc, "--std="+filepath.Join(root, "std"), "--wd="+tmp, "--play", "repl", filepath.Join(tmp, "warm"), filepath.Join(tmp, "sess")))
	c.Dir = tmp
	c.Stdin = strings.NewReader(in)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("--play repl: %v\n%s", err, out)
	}
	return out
}

// TestPlayReplConc: the playground's REPL builds an entry once, with the
// unwinding checks the last build used, handing the SSA builder its
// checker (decision 0583, 2026-11-12); an entry that needs the other
// setting is checked again and built with it. A Mutex binding turns them
// on, :reset off again, and every entry still builds.
func TestPlayReplConc(t *testing.T) {
	if testing.Short() || selfhostTier(t) == tierShort {
		t.Skip("builds compiler/wasm (merge and full tiers)")
	}
	in := strings.Join([]string{
		`{"in": "1 + 2"}`,
		`{"ran": "", "code": 0}`,
		`{"in": "let mu = Mutex[Int].new(1)"}`,
		`{"ran": "", "code": 0}`,
		`{"in": "let ch = Chan[Int].new(1)"}`,
		`{"ran": "", "code": 0}`,
		`{"cmd": "reset"}`,
		`{"in": "[1, 2].len()"}`,
		`{"ran": "", "code": 0}`,
	}, "\n") + "\n"
	out := playRepl(t, in)
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{`"run":`, `"type":"Int"`, `"run":`, `"type":"Mutex[Int]"`, `"run":`, `"type":"Chan[Int]"`, `"ok":true`, `"run":`, `"type":"Int"`}
	if len(got) != len(want) {
		t.Fatalf("--play repl: %d replies, want %d:\n%s", len(got), len(want), out)
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) || (w != `"run":` && !strings.Contains(got[i], `"ok":true`)) {
			t.Errorf("reply %d: %s, want %s and ok", i+1, got[i], w)
		}
	}
}

// playCompiler builds compiler/wasm natively into dir; it returns the
// binary and the repository's root.
func playCompiler(t *testing.T, dir string) (string, string) {
	t.Helper()
	if testing.Short() || selfhostTier(t) == tierShort {
		t.Skip("builds compiler/wasm (merge and full tiers)")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	xoc := filepath.Join(dir, "xoc")
	cmd := testtmp.Guard(exec.Command(xo, "build", "-o", xoc, "compiler/wasm"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("xo build compiler/wasm: %v\n%s", err, b)
	}
	return xoc, root
}

// TestPlayServe: one `--play serve` process (compiler/wasm play.xo, the
// page's one compiler a visit) answers a run of requests as a fresh
// process answers each, with the program's file rewritten at the same
// path between requests as the page does. Same replies, same built
// modules, same formatted text: no state (a checker, std's record, an
// output buffer, a definition of the last program) carries from one
// request to the next.
func TestPlayServe(t *testing.T) {
	tmp := t.TempDir()
	xoc, root := playCompiler(t, tmp)
	src := filepath.Join(tmp, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "xo.mod"), []byte("module play\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const (
		withHelper = "fn helper() -> Int { 41 }\n\nfn main(os: Os) {\n  os.stdio.println(\"A ${helper() + 1}\")\n}\n"
		usesHelper = "fn main(os: Os) {\n  os.stdio.println(\"B ${helper()}\")\n}\n"
		syntaxErr  = "fn main(os: Os) {\n  let x = \n}\n"
		other      = "struct P {\n  x: Int\n}\n\nfn main(os: Os) {\n  let p = P{x: 3}\n  os.stdio.println(\"C ${p.x}\")\n}\n"
		unformat   = "fn main(os:Os){\nos.stdio.println(\"D\")\n}\n"
	)
	steps := []struct{ cmd, prog string }{
		{"build", withHelper},
		{"check", usesHelper}, // helper is gone: the check reports it
		{"build", usesHelper},
		{"check", syntaxErr},
		{"build", other},
		{"fmt", unformat},
		{"check", other}, // after diagnostics: none
		{"build", withHelper},
	}
	main := filepath.Join(src, "main.xo")
	args := func(i int, cmd, tag string) []string {
		switch cmd {
		case "build":
			return []string{"build", src, filepath.Join(tmp, fmt.Sprintf("%s-%d.wasm", tag, i))}
		case "fmt":
			return []string{"fmt", main}
		}
		return []string{"check", src}
	}
	// what a request leaves besides its reply: the built module, or the
	// formatted file
	result := func(i int, cmd, tag string) string {
		p := main
		if cmd == "build" {
			p = filepath.Join(tmp, fmt.Sprintf("%s-%d.wasm", tag, i))
		} else if cmd != "fmt" {
			return ""
		}
		b, _ := os.ReadFile(p)
		return fmt.Sprintf("%x", sha256.Sum256(b))
	}
	base := []string{"--std=" + filepath.Join(root, "std"), "--wd=" + tmp, "--play"}

	srv := testtmp.Guard(exec.Command(xoc, append(base, "serve")...))
	srv.Dir = tmp
	in, err := srv.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outp, err := srv.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	srv.Stderr = &stderr
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewReader(outp)
	served := make([][2]string, len(steps))
	for i, s := range steps {
		if err := os.WriteFile(main, []byte(s.prog), 0o644); err != nil {
			t.Fatal(err)
		}
		req, _ := json.Marshal(args(i, s.cmd, "serve"))
		if _, err := in.Write(append(req, '\n')); err != nil {
			t.Fatal(err)
		}
		line, err := lines.ReadString('\n')
		if err != nil {
			t.Fatalf("request %d %s: no reply: %v\n%s", i, req, err, stderr.String())
		}
		served[i] = [2]string{strings.TrimSpace(line), result(i, s.cmd, "serve")}
	}
	in.Close()
	if err := srv.Wait(); err != nil {
		t.Fatalf("--play serve: %v\n%s", err, stderr.String())
	}

	sawDiag := false
	for i, s := range steps {
		if err := os.WriteFile(main, []byte(s.prog), 0o644); err != nil {
			t.Fatal(err)
		}
		c := testtmp.Guard(exec.Command(xoc, append(base, args(i, s.cmd, "one")...)...))
		c.Dir = tmp
		out, err := c.Output()
		if err != nil {
			t.Fatalf("--play %s: %v\n%s", s.cmd, err, out)
		}
		want := [2]string{strings.TrimSpace(string(out)), result(i, s.cmd, "one")}
		if served[i] != want {
			t.Errorf("request %d (%s): served %q, a fresh process %q", i, s.cmd, served[i], want)
		}
		if strings.Contains(want[0], "XO") {
			sawDiag = true
		}
	}
	// the run exercised what could leak: diagnostics, and a later
	// request with none
	if !sawDiag || strings.Contains(served[6][0], "XO") {
		t.Errorf("the steps did not show a diagnostic and then none: %q", served)
	}
	for _, i := range []int{0, 4, 7} {
		if !strings.Contains(served[i][0], `"ok":true`) {
			t.Errorf("request %d did not build: %s", i, served[i][0])
		}
	}
	if !strings.Contains(served[2][0], `"ok":false`) {
		t.Errorf("request 2 (helper gone) built: %s", served[2][0])
	}
}
