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
// stages land). 128 of 145 since std/json's text is Xo (decision 0193
// step 4: native_json, text_table, codex_gaps, json_text).
const wasmMinPass = 167

// TestSelfhostWasm is stages 1 and 2 of decision 0179: compiler/wasm (built
// with the go backend) lowers every golden program of
// internal/llvm/testdata/golden to a WebAssembly module, Node runs it
// under WASI preview1 (compiler/wasm/run.mjs, what the playground's
// worker does), and its exit status, standard output, and fault lines
// must be want.txt's. A golden the backend does not support yet must say
// so (`== unsupported <why>`, no module) and is counted; any other
// difference fails. Programs that exit 0 run again with XO_LEAKCHECK=1
// and must free every allocation. Not under -short (it builds the
// checker and the SSA builder).
// wasmCCallRE matches the run time faults of what a WASI module cannot
// do: a C library platform call (decision 0232, std/native/sys_c.xo), a
// function address or a call through one (decision 0233: rt.fn_addr,
// rt.frame_addr, rt.call_c2, rt.call_c4), and the working directory
// (WASI preview1 has none: rt.fs_raw_getcwd gives "", so
// os.fs.cwd_path() reports Io "getcwd failed"; the playground's compiler
// passes --wd), and a file's permission bits or advisory lock (WASI
// preview1 has neither: compiler/wasm rt4.xo's fs_perm, fs_chmod, and
// fs_lock report it as an Io error), and a modification time before 1970
// (WASI timestamps are unsigned). The test counts them as
// unsupported and logs each. (The native runtime's entropy was one until
// the SSA builder sent rand.xo's entropy to the wasm runtime's
// random_get, gotchas/wasm-native-rand-no-entropy.md.)
var wasmCCallRE = regexp.MustCompile("the platform call `[A-Za-z0-9_]+` of c on another OS|the wasm backend does not support the call `rt\\.(fn_addr|frame_addr|call_c2|call_c4)`|msg: \"getcwd failed\"|msg: \"operation not supported \\(WASI has no file modes or locks\\)\"|msg: \"WASI has no times before 1970\"")

func TestSelfhostWasm(t *testing.T) {
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
		t.Fatalf("compiler/wasm: %v\n%s", err, stderr.String())
	}
	results := parseWasmRuns(string(res))
	runner := filepath.Join(root, "compiler", "wasm", "run.mjs")
	pass, unsupported, failed := 0, 0, 0
	var why []string
	for _, r := range rel {
		name := filepath.Base(r)
		line, ok := results[r]
		if !ok {
			t.Errorf("%s: no result from compiler/wasm", name)
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
		if m := wasmCCallRE.FindString(got.String()); m != "" && got.String() != string(want) {
			// a C library call (decision 0232) in a WASI module: std's
			// guard faults by design, as an unsupported call would
			unsupported++
			why = append(why, name+": "+m)
			continue
		}
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

// wasmFsWriteProg writes files through rt.fs_write (WASI path_open and
// fd_write): create, truncate, a file larger than one write, an empty
// file, and the errors of a missing directory and of a directory.
const wasmFsWriteProg = `fn main(os: Os) ! {
  let out = os.stdio
  os.fs.write("a.txt", b"hello wasm\n")?
  os.fs.write("a.txt", b"hi\n")?
  out.println("read ${os.fs.read_text("a.txt")?.quote()}")
  var big = BytesBuilder.new()
  var i = 0
  while i < 20000 {
    big.write_str("row ${i}\n")
    i += 1
  }
  let data = big.freeze()
  os.fs.write("big.txt", data)?
  let back = os.fs.read("big.txt")?
  out.println("big ${back.len()} of ${data.len()} same ${back == data}")
  os.fs.write("d/e.txt", b"")?
  out.println("empty ${os.fs.read("d/e.txt")?.len()}")
  match os.fs.write("missing/x.txt", b"x") {
    Ok(_) => out.println("missing: written")
    Err(e) => out.println("missing: ${e}")
  }
  match os.fs.write("d", b"x") {
    Ok(_) => out.println("dir: written")
    Err(e) => out.println("dir: ${e}")
  }
}
`

// wasmFsWriteWant is what the go backend prints for wasmFsWriteProg.
const wasmFsWriteWant = `read "hi\n"
big 188890 of 188890 same true
empty 0
missing: NotFound(path: "missing/x.txt")
dir: IsDir(path: "d")
`

// TestSelfhostWasmFsWrite is stage 3's rt.fs_write (decision 0179): a
// program built by compiler/wasm writes files under Node's WASI in its
// working directory, prints what the go backend prints, and frees every
// allocation; the files hold what was written.
func TestSelfhostWasmFsWrite(t *testing.T) {
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
	src := filepath.Join(t.TempDir(), "fsw")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(wasmFsWriteProg), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	cmd := exec.Command(bin, "--std="+filepath.Join(root, "std"), "--wd="+src, "--out="+out, ".")
	cmd.Dir = src
	res, err := cmd.CombinedOutput()
	if err != nil || !strings.HasPrefix(string(res), "== ok ") {
		t.Fatalf("compiler/wasm: %v\n%s", err, res)
	}
	run := t.TempDir()
	if err := os.Mkdir(filepath.Join(run, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := runWasm(t, node, filepath.Join(root, "compiler", "wasm", "run.mjs"), filepath.Join(out, "fsw.wasm"), run, true)
	if got.exit != 0 || got.stdout != wasmFsWriteWant {
		t.Fatalf("got %s\nwant stdout\n%s", got, wasmFsWriteWant)
	}
	if m := wasmLeakRE.FindStringSubmatch(got.rawStderr); m == nil || m[1] != "0" {
		t.Errorf("leak check: %q", got.rawStderr)
	}
	if b, err := os.ReadFile(filepath.Join(run, "a.txt")); err != nil || string(b) != "hi\n" {
		t.Errorf("a.txt: %q %v", b, err)
	}
	if st, err := os.Stat(filepath.Join(run, "big.txt")); err != nil || st.Size() != 188890 {
		t.Errorf("big.txt: %v %v", st, err)
	}
}

var wasmTimeRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z`)

var wasmLeakRE = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// parseWasmRuns maps each run of `compiler/wasm --each` to its result line.
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
// (stableStderr there): timestamps as <time> (std/log's records), no
// indented lines, no crash bundle paths, no leak check line.
func stableWasmStderr(s string) string {
	s = wasmTimeRE.ReplaceAllString(s, "<time>")
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l == "" || strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t") || strings.HasPrefix(l, "crash bundle:") || strings.HasPrefix(l, "xo leakcheck:") || strings.HasPrefix(l, "xo wasm seed:") {
			continue
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}
