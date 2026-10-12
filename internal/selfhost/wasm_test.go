package selfhost

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// wasmMinPass is the number of LLVM goldens the WebAssembly backend must
// run correctly (stage 2 of decision 0179: 116 of 134; raise it as
// stages land).
const wasmMinPass = 116

// TestSelfhostWasm is stages 1 and 2 of decision 0179: selfhost/wasm (built
// with the go backend) lowers every golden program of
// internal/llvm/testdata/golden to a WebAssembly module, Node runs it
// under WASI preview1 (selfhost/wasm/run.mjs, what the playground's
// worker does), and its exit status, standard output, and fault lines
// must be want.txt's. A golden the backend does not support yet must say
// so (`== unsupported <why>`, no module) and is counted; any other
// difference fails. Programs that exit 0 run again with XO_LEAKCHECK=1
// and must free every allocation. Not under -short (it builds the
// checker and the SSA builder).
func TestSelfhostWasm(t *testing.T) {
	if selfhostTier(t) == tierShort {
		t.Skip("builds selfhost/wasm (merge and full tiers)")
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
	dirs, err := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no goldens: %v", err)
	}
	out := t.TempDir()
	var rel []string
	for _, d := range dirs {
		r, _ := filepath.Rel(root, d)
		rel = append(rel, r)
	}
	cmd := exec.Command(bin, append([]string{"--std=std", "--wd=" + root, "--each", "--out=" + out}, rel...)...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	res, err := cmd.Output()
	if err != nil {
		t.Fatalf("selfhost/wasm: %v\n%s", err, stderr.String())
	}
	results := parseWasmRuns(string(res))
	runner := filepath.Join(root, "selfhost", "wasm", "run.mjs")
	pass, unsupported, failed := 0, 0, 0
	var why []string
	for _, r := range rel {
		name := filepath.Base(r)
		line, ok := results[r]
		if !ok {
			t.Errorf("%s: no result from selfhost/wasm", name)
			failed++
			continue
		}
		if !strings.HasPrefix(line, "== ok ") {
			if strings.HasPrefix(line, "== unsupported ") {
				unsupported++
				why = append(why, name+": "+strings.TrimPrefix(line, "== unsupported "))
				continue
			}
			t.Errorf("%s: %s", name, line)
			failed++
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
			t.Errorf("%s: the wasm module's run differs from want.txt\n--- got\n%s--- want\n%s", name, got.String(), want)
			failed++
			continue
		}
		if got.exit == 0 {
			lk := runWasm(t, node, runner, wasm, dir, true)
			if m := wasmLeakRE.FindStringSubmatch(lk.rawStderr); m == nil || m[1] != "0" {
				t.Errorf("%s: leak check: %q", name, lk.rawStderr)
				failed++
				continue
			}
		}
		pass++
	}
	t.Logf("wasm goldens: %d pass, %d unsupported, %d fail of %d", pass, unsupported, failed, len(rel))
	for _, w := range why {
		t.Logf("unsupported %s", w)
	}
	if pass < wasmMinPass {
		t.Errorf("%d goldens pass, want at least %d (wasmMinPass)", pass, wasmMinPass)
	}
}

var wasmLeakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// parseWasmRuns maps each run of `selfhost/wasm --each` to its result line.
func parseWasmRuns(s string) map[string]string {
	out := map[string]string{}
	cur := ""
	for _, l := range strings.Split(s, "\n") {
		if d, ok := strings.CutPrefix(l, "=== wasm "); ok {
			cur = d
			continue
		}
		if strings.HasPrefix(l, "== ") && cur != "" {
			out[cur] = l
			cur = ""
		}
	}
	return out
}

type wasmRun struct {
	exit      int
	stdout    string
	stderr    string
	rawStderr string
}

func (r wasmRun) String() string {
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", r.exit, r.stdout, r.stderr)
}

// runWasm runs module wasm under Node in dir with the golden's args.txt,
// env.txt, and stdin.txt, as internal/llvm's golden test runs a binary.
func runWasm(t *testing.T, node, runner, wasm, dir string, leak bool) wasmRun {
	t.Helper()
	args := []string{"--no-warnings", runner, wasm}
	if data, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(data))...)
	}
	cmd := exec.Command(node, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if data, err := os.ReadFile(filepath.Join(dir, "env.txt")); err == nil {
		cmd.Env = append(cmd.Env, strings.Fields(string(data))...)
	}
	if leak {
		cmd.Env = append(cmd.Env, "XO_LEAKCHECK=1")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "stdin.txt")); err == nil {
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("node: %v", err)
	}
	return wasmRun{exit: code, stdout: stdout.String(), stderr: stableWasmStderr(stderr.String()), rawStderr: stderr.String()}
}

// stableWasmStderr keeps the lines internal/llvm's golden test keeps
// (stableStderr there): no indented lines, no crash bundle paths, no leak
// check line.
func stableWasmStderr(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "crash bundle:") || strings.HasPrefix(l, "xo leakcheck:") {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}
