package selfhost

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/runfilter"
	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestWasmCommandGoldens reruns the wasm goldens on wasip2/wasm (decision 0447
// stage 3): each golden TestSelfhostWasm runs is built with `xo build
// --targets=wasip2/wasm` as a wasi:cli/command component, checked with
// `wasm-tools validate` and run with `wasmtime run`, its stdin, arguments and
// environment as the golden gives them, against want_wasm.txt (else
// want.txt). wasi:cli/exit has only ok and err, so a golden whose status is
// not 0 must exit 1. A golden that does not build or run is an error unless
// its line in testdata/wasm_unsupported.txt names wasip2 among its targets
// (wasmLimits), and such a golden that passes is an error too. It skips where wasmtime or wasm-tools
// is not installed (the Mac); the linux/arm64 offload hosts have them.
// XO_WASM_GOLDENS chooses goldens as in TestSelfhostWasm.
func TestWasmCommandGoldens(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	needWasmTools(t, "wasm-tools", "wasmtime")
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	known := wasmLimits(t, filepath.Join(root, "internal", "selfhost", "testdata", "wasm_unsupported.txt"), "wasip2")
	dirs, err := filepath.Glob(filepath.Join(root, "internal", "llvm", "testdata", "golden", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no goldens: %v", err)
	}
	pats := os.Getenv("XO_WASM_GOLDENS")
	sel, filtered := runfilter.Sub(t)
	env := append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "XO_BIN_CACHE=off")
	jobs := runtime.NumCPU() / 2
	if jobs < 1 {
		jobs = 1
	}
	sem := make(chan struct{}, jobs)
	var mu sync.Mutex
	failed := map[string]string{}
	ran := map[string]bool{}
	t.Run("goldens", func(t *testing.T) {
		for _, d := range dirs {
			name := filepath.Base(d)
			// every golden, those preview 1 cannot run too (a wasip2
			// command has sockets, decision 0447): the list says which fail
			if !sel(name) || !matchAny(pats, name) {
				continue
			}
			if _, err := os.Stat(filepath.Join(d, "main.xo")); err != nil {
				continue
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				sem <- struct{}{}
				defer func() { <-sem }()
				why := commandGolden(t, xo, env, d)
				mu.Lock()
				ran[name] = true
				if why != "" {
					failed[name] = why
				}
				mu.Unlock()
				_, listed := known[name]
				if why != "" && !listed {
					t.Errorf("%s; fix it, or, when WASI 0.2 lacks what it uses, list wasip2 among its targets in internal/selfhost/testdata/wasm_unsupported.txt", why)
				} else if why != "" {
					// what fails now, beside the list's reason (-v)
					t.Logf("WASI limit (%s): %s", known[name], why)
				} else if listed {
					t.Logf("listed (%s) but passes", known[name])
				}
			})
		}
	})
	if pats != "" || filtered {
		return
	}
	for name := range known {
		if ran[name] && failed[name] == "" {
			t.Errorf("golden %s passes on wasip2/wasm: drop wasip2 from its targets in testdata/wasm_unsupported.txt", name)
		}
	}
}

// commandGoldenTimeout bounds one golden's run.
const commandGoldenTimeout = 120 * time.Second

// commandGolden builds and runs golden dir d as a command: "" when it
// matches its want file, else why not.
func commandGolden(t *testing.T, xo string, env []string, d string) string {
	comp := filepath.Join(t.TempDir(), "c.wasm")
	build := testtmp.Guard(exec.Command(xo, "build", "--targets=wasip2/wasm", "-o", comp, "."))
	build.Dir, build.Env = d, env
	if b, err := build.CombinedOutput(); err != nil {
		return "build: " + firstLine(string(b))
	}
	if b, err := testtmp.Guard(exec.Command("wasm-tools", "validate", comp)).CombinedOutput(); err != nil {
		return "wasm-tools validate: " + firstLine(string(b))
	}
	// the stack the Node runner gives the preview 1 goldens (run.mjs: a
	// worker with a 1 GiB stack) for their deep recursion; wasmtime runs a
	// component on an async stack, which must hold the wasm stack
	args := []string{"run", "-W", "max-wasm-stack=1073741824,async-stack-size=1077936128"}
	// the preopens of the Node runner (run.mjs): the golden's directory as
	// "." and the root as "/" (rt8.xo's descriptors 3 and 4)
	args = append(args, "--dir", ".::.", "--dir", "/::/")
	// its environment (cmd.Env below) and argv[0], the program's path
	// (with no cache of wasmtime's own, which would follow the golden's HOME)
	args = append(args, "-S", "inherit-env=y", "--argv0", comp, "-C", "cache=n")
	// the network only for a component that imports wasi:sockets (decision
	// 0447: least privilege, decided from its imports, not its source)
	wit, err := testtmp.Guard(exec.Command("wasm-tools", "component", "wit", comp)).CombinedOutput()
	if err != nil {
		return "wasm-tools component wit: " + firstLine(string(wit))
	}
	args = append(args, wasmNetFlags(string(wit))...)
	// the environment as runWasm gives the Node runner: the test's, then
	// env.txt's (a later value wins), inherited by the program
	runEnv := os.Environ()
	if data, err := os.ReadFile(filepath.Join(d, "env.txt")); err == nil {
		runEnv = append(runEnv, strings.Fields(string(data))...)
	}
	args = append(args, comp)
	if data, err := os.ReadFile(filepath.Join(d, "args.txt")); err == nil {
		args = append(args, strings.Fields(string(data))...)
	}
	cmd := testtmp.Guard(exec.Command("wasmtime", args...))
	cmd.Dir, cmd.Env = d, runEnv
	if data, err := os.ReadFile(filepath.Join(d, "stdin.txt")); err == nil {
		cmd.Stdin = bytes.NewReader(data)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		return "wasmtime: " + err.Error()
	}
	// a golden that never exits fails by name instead of holding the suite
	// (a server waiting on a connection, 2026-11-03)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-time.After(commandGoldenTimeout):
		_ = cmd.Process.Kill()
		<-done
		return "wasmtime: no exit in " + commandGoldenTimeout.String() + "\n--- stdout so far\n" + stdout.String()
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return "wasmtime: " + err.Error()
	}
	wantFile := filepath.Join(d, "want_wasm.txt")
	if _, err := os.Stat(wantFile); err != nil {
		wantFile = filepath.Join(d, "want.txt")
	}
	want, err := os.ReadFile(wantFile)
	if err != nil {
		return "no want file"
	}
	// wasi:cli/exit's err is status 1
	w := string(want)
	if m := wantExitRE.FindStringSubmatch(w); m != nil {
		if n, _ := strconv.Atoi(m[1]); n != 0 {
			w = "exit 1\n" + w[len(m[0]):]
		}
	}
	got := "exit " + strconv.Itoa(code) + "\n--- stdout\n" + stdout.String() + "--- stderr\n" + stableWasmStderr(stderr.String())
	if got != w {
		return "output differs\n--- got\n" + got + "--- want\n" + w
	}
	return ""
}

var wantExitRE = regexp.MustCompile(`^exit (\d+)\n`)

// wasmNetFlags are wasmtime's flags for the network a component's WIT
// (wasm-tools component wit) imports: none without wasi:sockets, the
// sockets for tcp or udp, name lookup for ip-name-lookup (decision 0447).
func wasmNetFlags(wit string) []string {
	var fl []string
	if strings.Contains(wit, "import wasi:sockets/tcp@") || strings.Contains(wit, "import wasi:sockets/udp@") {
		fl = append(fl, "-S", "inherit-network=y")
	}
	if strings.Contains(wit, "import wasi:sockets/ip-name-lookup@") {
		fl = append(fl, "-S", "allow-ip-name-lookup=y")
	}
	return fl
}

// listFile reads a list of golden names, one a line, with # comments.
// wasmLimits reads the WASI limits of wasm_unsupported.txt that name the
// target (wasip1 or wasip2): each golden's `AREA: REASON`, by name. A line
// not in the form `NAME # TARGETS # AREA: REASON` fails the test.
func wasmLimits(t *testing.T, path, target string) map[string]string {
	out := map[string]string{}
	for name, rest := range listFile(t, path) {
		targets, why, ok := strings.Cut(rest, "#")
		area, _, ok2 := strings.Cut(why, ":")
		area = strings.TrimSpace(area)
		ts := strings.Fields(targets)
		if !ok || !ok2 || len(ts) == 0 || !(area == "network" || area == "processes" || area == "files" || area == "c") {
			t.Fatalf("%s: line %q: want `NAME # TARGETS # AREA: REASON` (TARGETS wasip1 and/or wasip2; AREA network, processes, files or c)", path, name+" # "+rest)
		}
		for _, x := range ts {
			if x != "wasip1" && x != "wasip2" {
				t.Fatalf("%s: line %q: unknown target %q (want wasip1, wasip2)", path, name, x)
			}
			if x == target {
				out[name] = strings.TrimSpace(why)
			}
		}
	}
	return out
}

func listFile(t *testing.T, path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out
		}
		t.Fatal(err)
	}
	for _, l := range strings.Split(string(b), "\n") {
		name, why, _ := strings.Cut(l, "#")
		if name = strings.TrimSpace(name); name != "" {
			out[name] = strings.TrimSpace(why)
		}
	}
	return out
}

func matchAny(pats, name string) bool {
	if pats == "" {
		return true
	}
	for _, p := range strings.Split(pats, ",") {
		if ok, _ := filepath.Match(strings.TrimSpace(p), name); ok {
			return true
		}
	}
	return false
}

// needWasmTools skips t when a tool is not on PATH (the Mac), or fails it
// when XO_TEST_WASM_TOOLS=1: the lead's tier sets it, so its wasm command
// tests cannot pass by skipping (they did on the second linux/arm64 host,
// whose jobs had no /work/tools/bin on PATH, until 2026-11-03).
func needWasmTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			if os.Getenv("XO_TEST_WASM_TOOLS") == "1" {
				t.Fatalf("%s not found and XO_TEST_WASM_TOOLS=1", tool)
			}
			t.Skipf("%s not found", tool)
		}
	}
}
