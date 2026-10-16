package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// nativeBackends lists the native backends this host can build tests
// with.
func nativeBackends(t *testing.T) []string {
	t.Helper()
	var out []string
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		if _, err := xotool.CC(); err == nil {
			out = append(out, xotool.BackendArm64)
		}
	}
	if _, err := xotool.Clang(); err == nil {
		out = append(out, xotool.BackendLLVM)
	}
	if len(out) == 0 {
		t.Skip("no native backend on this host")
	}
	return out
}

func xoTest(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(t, append([]string{"test"}, args...), &out, &errOut)
	return code, out.String() + errOut.String()
}

// TestNoGCTests runs native test binaries (`xo test --no-gc`, decision
// 0083): the report of testdata/nogc/same, examples/notes-service, and
// selfhost/check (MemFs symbolic links) is the Go backend's, line for
// line; testdata/nogc/native (error traces, fault frames, and deadlock
// details, which native builds do not record) matches its want.txt.
func TestNoGCTests(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries on every backend")
	}
	backends := nativeBackends(t)
	// one build directory for the parallel subtests (each builds its own
	// program; t.Setenv is not allowed in parallel tests)
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	// the same report on every backend: no traceback (xo test turns it on;
	// LLVM's darwin builds have no line tables, and the Go backend prints
	// none; TestTraceback checks it)
	setenv(t, "XO_TRACEBACK", "0")
	gen := filepath.Join("..", "..", "internal", "gen", "testdata", "golden")
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("testdata", "nogc", "prop"), filepath.Join("..", "..", "examples", "notes-service"), filepath.Join("..", "..", "selfhost", "check"), filepath.Join(gen, "test_stdin_stop"), filepath.Join(gen, "test_signal_after"), filepath.Join(gen, "test_fakes"), filepath.Join(gen, "test_map_until"), filepath.Join(gen, "test_run_main_void"), filepath.Join(gen, "test_once"), filepath.Join(gen, "test_logs"), filepath.Join(gen, "test_shutdown"), filepath.Join(gen, "test_cli"), filepath.Join(gen, "test_term"), filepath.Join(gen, "test_proc_cmd"), filepath.Join(gen, "test_net_only"), filepath.Join(gen, "test_unix_kill")} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			goCode, goOut := xoTest(t, "-v", dir)
			for _, be := range backends {
				code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
				if code != goCode || out != goOut {
					t.Errorf("%s on %s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", dir, be, goCode, code, goOut, be, out)
				}
			}
		})
	}
	t.Run("real", func(t *testing.T) {
		// `xo test --real` (decision 0120), and the same tests skipped
		// without it
		t.Parallel()
		dir := filepath.Join("testdata", "nogc", "real")
		for _, args := range [][]string{{"-v", "--real", dir}, {"-v", dir}} {
			goCode, goOut := xoTest(t, args...)
			for _, be := range backends {
				code, out := xoTest(t, append([]string{"--no-gc", "--backend=" + be}, args...)...)
				if code != goCode || out != goOut {
					t.Errorf("%v on %s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", args, be, goCode, code, goOut, be, out)
				}
			}
		}
	})
	t.Run("native", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join("testdata", "nogc", "native")
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		for _, be := range backends {
			code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
			if code != 1 || out != string(want) {
				t.Errorf("%s on %s: exit %d\n--- want\n%s--- got\n%s", dir, be, code, want, out)
			}
		}
	})
}

var leakLine = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// testBlocks is the number of `test` blocks in the .xo files of dir (not
// below it): the tests a native test binary of dir numbers.
func testBlocks(t *testing.T, dir string) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.xo"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	re := regexp.MustCompile(`(?m)^test "`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n += len(re.FindAll(src, -1))
	}
	if n == 0 {
		t.Fatalf("no test blocks in %s", dir)
	}
	return n
}

// TestNoGCTestLeaks runs every passing test of the native test binaries
// with XO_LEAKCHECK=1: no allocation may be live at exit, one process per
// test and all of them in one batched process.
func TestNoGCTestLeaks(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	backends := nativeBackends(t)
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("testdata", "nogc", "prop"), filepath.Join("..", "..", "examples", "notes-service")} {
		n := testBlocks(t, dir)
		for _, be := range backends {
			res, err := xotool.Build([]string{dir}, xotool.Options{Mode: xotool.ModeTest, NoGC: true, Backend: be, Out: filepath.Join(t.TempDir(), "tests")})
			if err != nil || res.Failed {
				t.Fatalf("%s on %s: %v %s", dir, be, err, res.Output)
			}
			bin := res.Binaries[xotool.Host()]
			// the test binary numbers the test blocks of its root modules
			// from 0 (contract fuzz tests are not in a native binary);
			// the tests run in the module's directory
			tdir, _ := filepath.Abs(dir)
			passed := make([]bool, n)
			t.Run(fmt.Sprintf("%s/%s", filepath.Base(dir), be), func(t *testing.T) {
				for index := 0; index < n; index++ {
					// one process per test: they run in parallel
					t.Run(strconv.Itoa(index), func(t *testing.T) {
						t.Parallel()
						cmd := command(t, bin)
						cmd.Dir = tdir
						cmd.Env = append(cmd.Env, "XO_TEST="+strconv.Itoa(index), "XO_LEAKCHECK=1")
						var errb bytes.Buffer
						cmd.Stdout, cmd.Stderr = io.Discard, &errb
						if cmd.Run() != nil {
							return // a failing test exits without the check
						}
						passed[index] = true
						m := leakLine.FindStringSubmatch(errb.String())
						if m == nil {
							t.Errorf("%s test %d on %s: no leak check line:\n%s", dir, index, be, errb.String())
							return
						}
						if m[1] != "0" {
							t.Errorf("%s test %d on %s: %s allocations live at exit (of %s)", dir, index, be, m[1], m[2])
						}
					})
				}
			})
			// the passing tests in one process (XO_TEST_BATCH, decision
			// 0235): every run ends, the world reset between them frees
			// what each kept, and nothing is live at exit
			var runs []string
			for index, ok := range passed {
				if ok {
					runs = append(runs, strconv.Itoa(index)+"/1")
				}
			}
			if len(runs) == 0 {
				continue
			}
			cmd := command(t, bin)
			cmd.Dir = tdir
			cmd.Env = append(cmd.Env, "XO_TEST=0", "XO_TEST_BATCH="+strings.Join(runs, ","), "XO_LEAKCHECK=1")
			var outb, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &outb, &errb
			if err := cmd.Run(); err != nil {
				t.Errorf("%s batch on %s: %v\n%s%s", dir, be, err, outb.String(), errb.String())
				continue
			}
			if got := strings.Count(outb.String(), "XO-TEST-END\n"); got != len(runs) {
				t.Errorf("%s batch on %s: %d of %d runs ended\n%s", dir, be, got, len(runs), outb.String())
			}
			if m := leakLine.FindStringSubmatch(errb.String()); m == nil || m[1] != "0" {
				t.Errorf("%s batch on %s: leak check: %q", dir, be, errb.String())
			}
			if filepath.Base(dir) != "prop" {
				continue
			}
			// a serving binary (XO_TEST_SERVE, shrinking over a pipe):
			// with an empty best every replay is not smaller, so each
			// run skips its body after its inputs are made, and those
			// inputs are freed
			cmd = command(t, bin)
			cmd.Dir = tdir
			cmd.Env = append(cmd.Env, "XO_TEST=0", "XO_TEST_SERVE=1", "XO_LEAKCHECK=1")
			cmd.Stdin = strings.NewReader("0/1/r=1,1/b=\n0/1/r=2,2/b=\n0/1/r=3,3/b=\n")
			outb.Reset()
			errb.Reset()
			cmd.Stdout, cmd.Stderr = &outb, &errb
			if err := cmd.Run(); err != nil {
				t.Errorf("%s serve on %s: %v\n%s%s", dir, be, err, outb.String(), errb.String())
				continue
			}
			if got, skips := strings.Count(outb.String(), "XO-TEST-END\n"), strings.Count(outb.String(), "XO-PROP-NOT-SMALLER\n"); got != 3 || skips != 3 {
				t.Errorf("%s serve on %s: %d runs ended, %d not smaller, want 3 and 3\n%s", dir, be, got, skips, outb.String())
			}
			if m := leakLine.FindStringSubmatch(errb.String()); m == nil || m[1] != "0" {
				t.Errorf("%s serve on %s: leak check: %q", dir, be, errb.String())
			}
		}
	}
}

// notesSmoke drives a running examples/notes-service at base (token t0p,
// password "correct horse") and returns the normalized exchanges.
func notesSmoke(t *testing.T, base string) string {
	t.Helper()
	var b strings.Builder
	timeRE := regexp.MustCompile(`"(created|expires)":"[^"]*"`)
	tokRE := regexp.MustCompile(`"token":"[^"]*"`)
	do := func(method, path, token, body string) string {
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		text := tokRE.ReplaceAllString(timeRE.ReplaceAllString(string(data), `"$1":"T"`), `"token":"TOK"`)
		fmt.Fprintf(&b, "%s %s: %d %s\n", method, path, resp.StatusCode, text)
		return string(data)
	}
	do("GET", "/health", "", "")
	do("GET", "/notes", "", "")
	do("POST", "/notes", "t0p", `{"body":"buy milk","ttl_secs":60}`)
	do("POST", "/notes", "t0p", `{"body":""}`)
	do("POST", "/notes", "t0p", `not json`)
	do("GET", "/notes", "t0p", "")
	do("GET", "/notes/1", "t0p", "")
	do("GET", "/notes/abc", "t0p", "")
	do("DELETE", "/notes/1", "t0p", "")
	do("GET", "/notes/1", "t0p", "")
	do("POST", "/login", "", `{"name":"admin","password":"wrong"}`)
	login := do("POST", "/login", "", `{"name":"admin","password":"correct horse"}`)
	tok := regexp.MustCompile(`"token":"([^"]*)"`).FindStringSubmatch(login)
	if tok == nil {
		t.Fatalf("no token in %q", login)
	}
	do("GET", "/notes", tok[1], "")
	do("GET", "/notes", "t0p", "")
	return b.String()
}

// TestNoGCNotesServer builds examples/notes-service with the Go backend
// and each native backend, serves it on a real socket, drives its API,
// stops it with SIGTERM, and requires the same answers and a clean
// shutdown (exit 0 after "stopped").
func TestNoGCNotesServer(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs the service")
	}
	backends := append([]string{xotool.BackendGo}, nativeBackends(t)...)
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := filepath.Join("..", "..", "examples", "notes-service")
	var want string
	for _, be := range backends {
		opts := xotool.Options{Backend: be, NoGC: be != xotool.BackendGo, Out: filepath.Join(t.TempDir(), "notes")}
		res, err := xotool.Build([]string{dir}, opts)
		if err != nil || res.Failed {
			t.Fatalf("%s: %v %s", be, err, res.Output)
		}
		cmd := command(t, res.Binaries[xotool.Host()])
		testtmp.Lifeline(t, cmd, 30*time.Minute) // no orphan if the test is killed
		cmd.Env = append(cmd.Env, "NOTES_TOKEN=t0p", "NOTES_PASSWORD=correct horse", "NOTES_ADDR=127.0.0.1:0")
		stderr, err := cmd.StderrPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		lines := make(chan string, 64)
		go func() {
			sc := bufio.NewScanner(stderr)
			for sc.Scan() {
				lines <- sc.Text()
			}
			close(lines)
		}()
		addrRE := regexp.MustCompile(`listening addr=(\S+)`)
		var addr string
		var logs []string
		deadline := time.After(20 * time.Second)
		for addr == "" {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("%s: exited before listening: %v", be, logs)
				}
				logs = append(logs, l)
				if m := addrRE.FindStringSubmatch(l); m != nil {
					addr = m[1]
				}
			case <-deadline:
				cmd.Process.Kill()
				t.Fatalf("%s: no listening line", be)
			}
		}
		got := notesSmoke(t, "http://"+addr)
		// the process this test started (its own PID)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		for l := range lines {
			logs = append(logs, l)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("%s: exit: %v (log %v)", be, err, logs)
		}
		if last := logs[len(logs)-1]; !strings.HasSuffix(last, "INFO stopped") {
			t.Errorf("%s: last log line %q", be, last)
		}
		if be == xotool.BackendGo {
			want = got
		} else if got != want {
			t.Errorf("%s answers differ from the Go backend's\n--- go\n%s--- %s\n%s", be, want, be, got)
		}
	}
}

// TestNoGCRunCache: the run cache of native `xo test` (decision 0236,
// selfhost/xo testcache.xo) reuses an unchanged program's binary and test
// list, and an edit, a new file, or a changed xo.mod misses it.
func TestNoGCRunCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	for _, be := range backends {
		t.Run(be, func(t *testing.T) {
			dir := filepath.Join(tmp, be)
			write := func(name, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			write("xo.mod", "module runcache\n")
			write("main.xo", "fn add(a: Int, b: Int) -> Int { a + b }\n\ntest \"add\" {\n  expect(add(1, 2)) == 3\n}\n")
			step := func(what string, wantCode int, wantHit bool, want string) {
				t.Helper()
				code, out := xoTest(t, "--no-gc", "--backend="+be, "--phase-times", dir)
				hit := strings.Contains(out, "phase run_cache ")
				if code != wantCode || hit != wantHit || !strings.Contains(out, want) {
					t.Fatalf("%s: exit %d (want %d), cache hit %v (want %v), want %q in\n%s", what, code, wantCode, hit, wantHit, want, out)
				}
			}
			step("first run", 0, false, "1 passed")
			step("unchanged", 0, true, "1 passed")
			write("main.xo", "fn add(a: Int, b: Int) -> Int { a - b }\n\ntest \"add\" {\n  expect(add(1, 2)) == 3\n}\n")
			step("edited", 1, false, "FAIL: add")
			step("edited, unchanged", 1, true, "FAIL: add")
			write("more.xo", "test \"more\" {\n  expect(2 + 2) == 4\n}\n")
			step("new file", 1, false, "1 passed, 1 failed")
			write("xo.mod", "module runcache\n\n")
			step("xo.mod changed", 1, false, "1 passed, 1 failed")
		})
	}
}

// TestNoGCTimeout runs testdata/nogc/hang, whose middle test never
// finishes, with `--timeout=2s`: on every backend the test fails with
// std/rt's message, the tests around it pass, and the reports are the
// Go backend's; natively in batches, one process per test, and with
// --timeout=0 (no limit) on the tests that finish.
func TestNoGCTimeout(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	backends := nativeBackends(t)
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := filepath.Join("testdata", "nogc", "hang")
	goCode, goOut := xoTest(t, "-v", "--timeout=2s", dir)
	for _, want := range []string{"--- PASS: before\n", "--- FAIL: spins (", "    the test did not finish within 2s of real time\n", "      2026-01-01T00:00:00Z INFO about to spin\n", "--- PASS: after\n", "FAIL: 2 passed, 1 failed, 0 skipped\n"} {
		if goCode != 1 || !strings.Contains(goOut, want) {
			t.Fatalf("go backend: exit %d, no %q in\n%s", goCode, want, goOut)
		}
	}
	for _, be := range backends {
		for _, batch := range []string{"1", "0"} {
			setenv(t, "XO_TEST_BATCH", batch)
			start := time.Now()
			code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", "--timeout=2s", dir)
			if code != goCode || out != goOut {
				t.Errorf("%s (XO_TEST_BATCH=%s) differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", be, batch, goCode, code, goOut, be, out)
			}
			if d := time.Since(start); d > 90*time.Second {
				t.Errorf("%s (XO_TEST_BATCH=%s): took %v", be, batch, d)
			}
		}
		code, out := xoTest(t, "--no-gc", "--backend="+be, "--timeout=0", "--run=fore", dir)
		if code != 0 || !strings.Contains(out, "ok: 1 passed, 0 failed, 0 skipped") {
			t.Errorf("%s --timeout=0: exit %d\n%s", be, code, out)
		}
	}
	code, out := xoTest(t, "--timeout=0", "--run=fore", dir)
	if code != 0 || !strings.Contains(out, "ok: 1 passed, 0 failed, 0 skipped") {
		t.Errorf("go --timeout=0: exit %d\n%s", code, out)
	}
}

// TestNoGCShrinkOverhead guards the harness's cost per test process and
// per shrinking replay: testdata/nogc/prop's 17 failing property tests
// make about 900 replays, and a warm `xo test` of it took about 0.5 s on
// arm64 when this was written (2026-10-15), 20 s while a watchdog polled
// every 20 ms per replay. A run above 5 s fails: an overhead grown about
// tenfold, which machine load alone does not explain.
func TestNoGCShrinkOverhead(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	if !slices.Contains(nativeBackends(t), xotool.BackendArm64) {
		t.Skip("measured on the arm64 backend")
	}
	setenv(t, "XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := filepath.Join("testdata", "nogc", "prop")
	args := []string{"--no-gc", "--backend=" + xotool.BackendArm64, dir}
	code, out := xoTest(t, args...) // builds
	if code != 1 || !strings.Contains(out, "FAIL: 3 passed, 17 failed, 0 skipped") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	best := time.Duration(1 << 62)
	for i := 0; i < 3; i++ {
		start := time.Now()
		xoTest(t, args...)
		best = min(best, time.Since(start))
	}
	if best > 5*time.Second {
		t.Errorf("a warm xo test of %s took %v (best of 3), want under 5s (about 0.5s on arm64, 2026-10-15)", dir, best)
	}
}
