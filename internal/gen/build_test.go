package gen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// concurrentGolden are the golden programs that start tasks; they are also
// run with the Go race detector.
// Directories named test_* are built in test mode.
var concurrentGolden = []string{"scope_tasks", "scope_failure", "channels", "mutex_atomic", "task_std", "cancellation", "map_value_semantics", "list_value_semantics", "arena_escape", "arena_holes", "test_shutdown", "test_signal_after", "chan_loops", "test_once", "test_map_until", "test_explore", "test_deadlock", "deadlock_real", "mutex_read", "test_mutex_read", "test_go_callback_task", "test_go_callback_deadlock", "go_callback_deadlock_real"}

// arenaRaceGolden are the golden programs also run with -race and
// `--no-arenas` (TestGoldenRaceNoArenas); TestGoldenRace builds with
// arenas (the default), and the servers among them hand arenas from one
// request goroutine to the next.
var arenaRaceGolden = []string{"arena_escape", "arena_holes", "http_real", "http_router", "mutex_atomic", "scope_tasks"}

// TestGoldenRace runs the concurrent golden programs built with -race and
// expects the same output and no data race reports.
func TestGoldenRace(t *testing.T) { testGoldenRace(t, concurrentGolden, false) }

// TestGoldenRaceNoArenas is TestGoldenRace for programs built with
// `--no-arenas`.
func TestGoldenRaceNoArenas(t *testing.T) { testGoldenRace(t, arenaRaceGolden, true) }

func testGoldenRace(t *testing.T, names []string, noArenas bool) {
	if testing.Short() {
		t.Skip("race builds are slow")
	}
	if out, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err != nil || strings.TrimSpace(string(out)) != "1" {
		t.Skip("the race detector needs cgo")
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("testdata", "golden", name)
			mode := xotool.ModeBuild
			if strings.HasPrefix(name, "test_") {
				mode = xotool.ModeTest
			}
			res, err := xotool.Build([]string{dir}, xotool.Options{Mode: mode, Race: true, NoArenas: noArenas})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			var args []string
			if data, err := os.ReadFile(filepath.Join(dir, "args.txt")); err == nil {
				args = strings.Fields(string(data))
			}
			cmd := exec.Command(res.Binaries[xotool.Host()], args...)
			cmd.Dir = dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			cmd.Run()
			if strings.Contains(stderr.String(), "DATA RACE") {
				t.Fatalf("data race:\n%s", stderr.String())
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(want), "--- stdout\n"+stdout.String()+"--- stderr") {
				t.Errorf("stdout differs under -race:\n%s\nwant.txt:\n%s", stdout.String(), want)
			}
		})
	}
}

// TestCrossCompile builds a program for linux/amd64, darwin/arm64,
// windows/amd64, and wasip1/wasm (core 12.1).
func TestCrossCompile(t *testing.T) {
	if testing.Short() {
		t.Skip("cross builds are slow")
	}
	dir := filepath.Join("testdata", "golden", "scope_tasks")
	out := t.TempDir()
	targets := []string{"linux/amd64", "darwin/arm64", "windows/amd64", "wasi/wasm32"}
	res, err := xotool.Build([]string{dir}, xotool.Options{Targets: strings.Join(targets, ","), Out: out})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, tg := range targets {
		bin := res.Binaries[tg]
		fi, err := os.Stat(bin)
		if err != nil || fi.Size() == 0 {
			t.Errorf("%s: no binary at %s", tg, bin)
		}
	}
	if len(res.Binaries) != 4 {
		t.Errorf("got %d binaries, want 4", len(res.Binaries))
	}
}

// TestFaultStackPointsAtXo checks that a fault's Go stack trace names the
// .xo file and line of every Xo frame (core 11.5, //line directives).
func TestFaultStackPointsAtXo(t *testing.T) {
	dir := filepath.Join("testdata", "golden", "arith_overflow")
	got, _ := buildAndRun(t, dir, xotool.ModeBuild)
	if got.exit != 70 {
		t.Fatalf("exit %d, want 70", got.exit)
	}
	// the fault happens in add_all (main.xo:4), called from main (main.xo:16)
	for _, want := range []string{`main\.xo:4\b`, `main\.xo:16\b`, `F_add_all`} {
		if !regexp.MustCompile(want).MatchString(got.stderr) {
			t.Errorf("stack trace does not contain %s:\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stderr, ".go:") {
		t.Errorf("stack trace mentions generated Go files:\n%s", got.stderr)
	}
}

// signalBinary builds testdata/signal once for the real signal tests.
var signalBinary = sync.OnceValues(func() (string, error) {
	res, err := xotool.Build([]string{filepath.Join("testdata", "signal")}, xotool.Options{})
	if err != nil {
		return "", err
	}
	if res.Failed {
		return "", fmt.Errorf("build failed:\n%s", res.Output)
	}
	return res.Binaries[xotool.Host()], nil
})

// signalChild is a started testdata/signal process. Only this test's own
// child is ever signaled, by the PID exec gave it.
type signalChild struct {
	t     *testing.T
	cmd   *exec.Cmd
	lines chan string
	out   strings.Builder
	done  chan struct{}
	stdin io.WriteCloser // with STDIN=pipe
}

func startSignalChild(t *testing.T, env ...string) *signalChild {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no SIGTERM on windows")
	}
	var stdinW io.WriteCloser
	bin, err := signalBinary()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	if slices.Contains(env, "STDIN=pipe") {
		// standard input stays open, so read_line blocks
		w, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { w.Close() })
		stdinW = w
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &signalChild{t: t, cmd: cmd, lines: make(chan string, 100), done: make(chan struct{}), stdin: stdinW}
	go func() {
		defer close(c.lines)
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				c.lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		select {
		case <-c.done:
		default:
			cmd.Process.Kill() // our own child, by its PID
			cmd.Wait()
		}
	})
	return c
}

// expect waits for the next line of output and checks it.
func (c *signalChild) expect(want string) {
	c.t.Helper()
	select {
	case line, ok := <-c.lines:
		if !ok {
			c.t.Fatalf("output ended, want %q (so far %q)", want, c.out.String())
		}
		c.out.WriteString(line)
		if line != want+"\n" {
			c.t.Fatalf("line %q, want %q (so far %q)", line, want, c.out.String())
		}
	case <-time.After(20 * time.Second):
		c.t.Fatalf("no line %q within 20s (so far %q)", want, c.out.String())
	}
}

func (c *signalChild) signal() {
	c.t.Helper()
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		c.t.Fatal(err)
	}
}

// wait waits for the process to exit and returns its status and the rest
// of its output.
func (c *signalChild) wait() (int, string) {
	c.t.Helper()
	var rest strings.Builder
	timeout := time.After(20 * time.Second)
	for {
		select {
		case line, ok := <-c.lines:
			if ok {
				rest.WriteString(line)
				continue
			}
			err := c.cmd.Wait()
			close(c.done)
			code := 0
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			} else if err != nil {
				c.t.Fatal(err)
			}
			return code, rest.String()
		case <-timeout:
			c.t.Fatalf("the program did not exit within 20s (output %q)", c.out.String()+rest.String())
		}
	}
}

// TestSignalTwoPhase sends SIGTERM to a running program (core 7.5): the
// first signal only requests a stop, the second cancels main's root scope
// (defer blocks run, main returns normally).
func TestSignalTwoPhase(t *testing.T) {
	c := startSignalChild(t)
	c.expect("ready")
	c.signal()
	c.expect("stop requested true")
	time.Sleep(300 * time.Millisecond)
	select {
	case line := <-c.lines:
		t.Fatalf("output after the first signal: %q", line)
	default:
	}
	c.signal()
	code, rest := c.wait()
	if code != 0 {
		t.Fatalf("exit %d (output %q)", code, rest)
	}
	for _, want := range []string{"background stopped", "cleanup ran"} {
		if !strings.Contains(rest, want) {
			t.Errorf("output %q lacks %q", rest, want)
		}
	}
	if strings.Contains(rest, "not reached") {
		t.Errorf("main continued after cancellation: %q", rest)
	}
}

// TestSignalDrain: a program that finishes its work after the stop
// request exits 0 without being canceled.
func TestSignalDrain(t *testing.T) {
	c := startSignalChild(t, "MODE=drain")
	c.expect("ready")
	c.signal()
	c.expect("stop requested true")
	c.expect("drained")
	code, rest := c.wait()
	if code != 0 || !strings.Contains(rest, "cleanup ran") {
		t.Fatalf("exit %d, output %q", code, rest)
	}
}

// TestSignalGrace: `os.proc.set_grace(300ms)` cancels a program that
// ignores the stop 300ms after the first signal.
func TestSignalGrace(t *testing.T) {
	c := startSignalChild(t, "MODE=grace")
	c.expect("ready")
	start := time.Now()
	c.signal()
	c.expect("stop requested true")
	code, rest := c.wait()
	el := time.Since(start)
	if code != 0 || !strings.Contains(rest, "cleanup ran") {
		t.Fatalf("exit %d, output %q", code, rest)
	}
	if el < 250*time.Millisecond || el > 10*time.Second {
		t.Errorf("canceled after %s, want about 300ms", el)
	}
}

// TestSignalThirdExits: a program stuck in cleanup that ignores
// cancellation exits with status 130 at the third signal.
func TestSignalThirdExits(t *testing.T) {
	c := startSignalChild(t, "MODE=hung")
	c.expect("ready")
	c.signal()
	c.expect("stop requested true")
	c.signal()
	time.Sleep(300 * time.Millisecond)
	c.signal()
	code, rest := c.wait()
	if code != 130 {
		t.Fatalf("exit %d, want 130 (output %q)", code, rest)
	}
}

// TestSignalStopsReadLine: a stop request ends a read_line blocked on
// standard input with IoErr.Canceled (core 7.5, decision 0088); the
// lines before it are read.
func TestSignalStopsReadLine(t *testing.T) {
	c := startSignalChild(t, "MODE=stdin", "STDIN=pipe")
	c.expect("ready")
	io.WriteString(c.stdin, "a\nb\n")
	time.Sleep(300 * time.Millisecond)
	c.signal()
	c.expect("read_line: Canceled after 2 lines")
	if code, rest := c.wait(); code != 0 {
		t.Fatalf("exit %d (output %q)", code, rest)
	}
}

// TestCancelStopsReadLine: cancelling the scope of a task blocked in
// read_line ends it with IoErr.Canceled (core 7.2, decision 0088).
func TestCancelStopsReadLine(t *testing.T) {
	c := startSignalChild(t, "MODE=stdin_cancel", "STDIN=pipe")
	c.expect("ready")
	c.expect("read_line: Canceled after 0 lines")
	if code, rest := c.wait(); code != 0 {
		t.Fatalf("exit %d (output %q)", code, rest)
	}
}

// TestSignalHTTPInFlight: after SIGTERM, `serve` stops accepting, finishes
// the request in flight, and returns Ok.
func TestSignalHTTPInFlight(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	c := startSignalChild(t, "MODE=http", fmt.Sprintf("PORT=%d", port))
	c.expect("ready")
	url := fmt.Sprintf("http://127.0.0.1:%d/slow", port)
	type result struct {
		status int
		body   string
		err    error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get(url)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		got <- result{status: resp.StatusCode, body: string(b), err: err}
	}()
	c.expect("handling")
	c.signal()
	r := <-got
	if r.err != nil || r.status != 200 || r.body != "slow done" {
		t.Fatalf("in flight request: %+v", r)
	}
	code, rest := c.wait()
	if code != 0 || rest != "serve returned\n" {
		t.Fatalf("exit %d, output %q", code, rest)
	}
	if _, err := http.Get(url); err == nil {
		t.Errorf("the server still accepts after the stop")
	}
}

// TestHoles builds a program with a hole under --holes: it runs until the
// hole is evaluated, which faults (core 3.7). Without --holes the build is
// rejected.
func TestHoles(t *testing.T) {
	dir := filepath.Join("testdata", "holes")
	res, err := xotool.Build([]string{dir}, xotool.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Fatalf("a program with a hole built without --holes")
	}
	res, err = xotool.Build([]string{dir}, xotool.Options{Holes: true})
	if err != nil || res.Failed {
		t.Fatalf("build --holes: %v %+v", err, res)
	}
	cmd := exec.Command(res.Binaries[xotool.Host()])
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 70 {
		t.Fatalf("exit %v, want 70", err)
	}
	if stdout.String() != "before hole 3\n" {
		t.Errorf("stdout %q", stdout.String())
	}
	for _, want := range []string{"[main.xo:4:11] 1 + 2 = 3", "evaluated a hole `???` at main.xo:1:26"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
}

// TestRunSeedReproduces runs a program on the deterministic scheduler with
// the real clock (XO_SEED, `xo run --seed=N`, core 7.4): the same seed
// prints the same interleaving every time, and different seeds give
// different interleavings.
func TestRunSeedReproduces(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	res, err := xotool.Build([]string{filepath.Join("testdata", "sched_run")}, xotool.Options{})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %+v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	runSeed := func(seed int) string {
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), fmt.Sprintf("XO_SEED=%d", seed))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, out)
		}
		return string(out)
	}
	distinct := map[string]bool{}
	for seed := 1; seed <= 8; seed++ {
		first := runSeed(seed)
		if !strings.HasSuffix(first, "total 18\n") {
			t.Fatalf("seed %d: output %q", seed, first)
		}
		for i := 0; i < 3; i++ {
			if again := runSeed(seed); again != first {
				t.Fatalf("seed %d printed\n%s\nthen\n%s", seed, first, again)
			}
		}
		distinct[first] = true
	}
	if len(distinct) < 4 {
		t.Errorf("8 seeds gave only %d interleavings", len(distinct))
	}
}

// TestTestSeedReproduces runs the explore golden's race under its failing
// seed several times: the same seed gives the same failure, byte for byte.
func TestTestSeedReproduces(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	dir := filepath.Join("testdata", "golden", "test_explore")
	res, err := xotool.Build([]string{dir}, xotool.Options{Mode: xotool.ModeTest})
	if err != nil || res.Failed {
		t.Fatalf("build: %v %+v", err, res)
	}
	bin := res.Binaries[xotool.Host()]
	runSeed := func(seed string) string {
		cmd := exec.Command(bin, "-seed", seed, "-run", "lost update", "-v")
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	for _, seed := range []string{"2", "3"} {
		first := runSeed(seed)
		for i := 0; i < 5; i++ {
			if again := runSeed(seed); again != first {
				t.Fatalf("seed %s printed\n%s\nthen\n%s", seed, first, again)
			}
		}
	}
	if !strings.Contains(runSeed("2"), "--- FAIL: lost update (main_test.xo:1:1, seed 2)") {
		t.Errorf("seed 2 does not fail:\n%s", runSeed("2"))
	}
	if strings.Contains(runSeed("3"), "--- FAIL: lost update (") {
		t.Errorf("seed 3 fails:\n%s", runSeed("3"))
	}
}
