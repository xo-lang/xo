package gen_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// Record and replay (core 11.3) and crash bundles (core 11.2): programs
// under testdata/replay are recorded with XO_RECORD (or XO_RING) and
// replayed with XO_REPLAY, offline: no standard input, no data files, no
// network. Each test also runs on a -race build when cgo is available.

// replayBuilds runs f for a plain build and, when the race detector is
// available and the run is not short, for a -race build.
func replayBuilds(t *testing.T, f func(t *testing.T, race bool)) {
	t.Run("plain", func(t *testing.T) { f(t, false) })
	if testing.Short() {
		return
	}
	if out, err := exec.Command("go", "env", "CGO_ENABLED").Output(); err != nil || strings.TrimSpace(string(out)) != "1" {
		return
	}
	t.Run("race", func(t *testing.T) { f(t, true) })
}

func buildReplayProg(t *testing.T, dir string, race bool) string {
	t.Helper()
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Race: race})
	if err != nil || res.Failed {
		t.Fatalf("build %s: %v %+v", dir, err, res)
	}
	return res.Binaries[build.Host()]
}

// progRun is one run of a built program.
type progRun struct {
	code           int
	stdout, stderr string
}

// runProg runs bin in dir with the extra environment and standard input.
func runProg(t *testing.T, bin, dir, stdin string, env ...string) progRun {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %s: %v", bin, err)
	}
	if strings.Contains(errb.String(), "DATA RACE") {
		t.Fatalf("data race:\n%s", errb.String())
	}
	return progRun{code, out.String(), errb.String()}
}

var (
	bundleLineRE = regexp.MustCompile(`(?m)^crash bundle: (.*)$`)
	replayLineRE = regexp.MustCompile(`(?m)^xo replay: .*\n`)
)

// programStderr is the program's own standard error: without the crash
// bundle line of the recording and the verdict of the replay.
func programStderr(s string) string {
	s = bundleLineRE.ReplaceAllString(s, "")
	s = replayLineRE.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, "\n\n", "\n")
}

func bundlePath(t *testing.T, stderr string) string {
	t.Helper()
	m := bundleLineRE.FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("no crash bundle in:\n%s", stderr)
	}
	return m[1]
}

// TestReplayEffects records a program that reads standard input, the
// clock, rand, the environment (a secret), and a file, then faults, and
// replays it offline: the same output and the same fault. The secret is
// redacted in the trace and the replay needs it from the environment.
// The crash bundle carries the trace and replays to the same fault.
func TestReplayEffects(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	replayBuilds(t, func(t *testing.T, race bool) {
		bin := buildReplayProg(t, filepath.Join("testdata", "replay", "effects"), race)
		work := t.TempDir()
		crash := t.TempDir()
		if err := os.WriteFile(filepath.Join(work, "data.txt"), []byte("hello world\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		trace := filepath.Join(t.TempDir(), "run.xotrace")
		const secret = "s3cret-token-value"
		recd := runProg(t, bin, work, "input line\n", "XO_RECORD="+trace, "XO_TEST_TOKEN="+secret, "XO_CRASH_DIR="+crash)
		if recd.code != 70 || !strings.Contains(recd.stdout, "file: hello world") || !strings.Contains(recd.stderr, "fault: index 22 out of range") {
			t.Fatalf("recording: exit %d\n%s\n%s", recd.code, recd.stdout, recd.stderr)
		}
		data, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(secret)) || !bytes.Contains(data, []byte("<redacted:1>")) {
			t.Fatalf("the secret is not redacted:\n%s", data)
		}
		for _, op := range []string{`"stdio.read_line"`, `"clock.now"`, `"rand.int"`, `"env.get"`, `"fs.read"`, `"clock.sleep"`, `{"s":`} {
			if !bytes.Contains(data, []byte(op)) {
				t.Errorf("the trace has no %s:\n%s", op, data)
			}
		}

		// offline: another directory without data.txt, no standard input
		empty := t.TempDir()
		missing := runProg(t, bin, empty, "", "XO_REPLAY="+trace)
		if missing.code != 3 || !strings.Contains(missing.stderr, `the value of env.get("XO_TEST_TOKEN") at main.xo:12`) || !strings.Contains(missing.stderr, "set XO_TEST_TOKEN (or XO_REPLAY_SECRET_1)") {
			t.Fatalf("replay without the secret: exit %d\n%s", missing.code, missing.stderr)
		}
		rep := runProg(t, bin, empty, "", "XO_REPLAY="+trace, "XO_TEST_TOKEN="+secret)
		if rep.code != recd.code || rep.stdout != recd.stdout || programStderr(rep.stderr) != programStderr(recd.stderr) {
			t.Fatalf("replay differs: exit %d (recorded %d)\n--- stdout\n%s--- recorded\n%s--- stderr\n%s--- recorded\n%s", rep.code, recd.code, rep.stdout, recd.stdout, rep.stderr, recd.stderr)
		}
		if !strings.Contains(rep.stderr, "xo replay: identical run: fault: index 22 out of range for List of length 3 at main.xo:18:30, exit status 70") {
			t.Errorf("replay verdict:\n%s", rep.stderr)
		}
		alt := runProg(t, bin, empty, "", "XO_REPLAY="+trace, "XO_REPLAY_SECRET_1="+secret)
		if alt.code != 70 || alt.stdout != recd.stdout {
			t.Errorf("replay with XO_REPLAY_SECRET_1: exit %d\n%s\n%s", alt.code, alt.stdout, alt.stderr)
		}

		// the crash bundle
		bundle := bundlePath(t, recd.stderr)
		var b map[string]any
		bdata, err := os.ReadFile(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(bdata, &b); err != nil {
			t.Fatalf("bundle: %v\n%s", err, bdata)
		}
		for _, k := range []string{"fault", "location", "stack", "tasks", "build", "target", "trace"} {
			if b[k] == nil {
				t.Errorf("the bundle has no %q:\n%s", k, bdata)
			}
		}
		if bi, _ := b["build"].(map[string]any); bi == nil || bi["id"] == nil || bi["sources"] == nil {
			t.Errorf("the bundle's build has no id or source hashes: %v", b["build"])
		}
		if bytes.Contains(bdata, []byte(secret)) {
			t.Errorf("the bundle holds the secret")
		}
		brep := runProg(t, bin, empty, "", "XO_REPLAY="+bundle, "XO_TEST_TOKEN="+secret)
		if brep.code != 70 || brep.stdout != recd.stdout || !strings.Contains(brep.stderr, "xo replay: reproduced the crash: fault: index 22 out of range") {
			t.Fatalf("bundle replay: exit %d\n%s\n%s", brep.code, brep.stdout, brep.stderr)
		}
	})
}

// TestReplayInterleave records a concurrent program whose output depends
// on the interleaving, random sleeps, and timers several times; each
// replay prints the same lines and ends the same way.
func TestReplayInterleave(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	replayBuilds(t, func(t *testing.T, race bool) {
		bin := buildReplayProg(t, filepath.Join("testdata", "replay", "interleave"), race)
		dir := t.TempDir()
		crash := t.TempDir()
		outputs := map[string]bool{}
		for i := 0; i < 6; i++ {
			trace := filepath.Join(dir, fmt.Sprintf("run%d.xotrace", i))
			// distinct seeds give distinct interleavings
			recd := runProg(t, bin, dir, "", "XO_RECORD="+trace, fmt.Sprintf("XO_SEED=%d", i+1), "XO_CRASH_DIR="+crash)
			if recd.code != 0 && recd.code != 70 {
				t.Fatalf("recording %d: exit %d\n%s", i, recd.code, recd.stderr)
			}
			outputs[recd.stdout] = true
			rep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+trace)
			if rep.code != recd.code || rep.stdout != recd.stdout || programStderr(rep.stderr) != programStderr(recd.stderr) {
				t.Fatalf("replay %d differs: exit %d (recorded %d)\n--- stdout\n%s--- recorded\n%s--- stderr\n%s", i, rep.code, recd.code, rep.stdout, recd.stdout, rep.stderr)
			}
			if !strings.Contains(rep.stderr, "xo replay: identical run") {
				t.Errorf("replay %d verdict:\n%s", i, rep.stderr)
			}
		}
		if len(outputs) < 2 {
			t.Errorf("6 seeds gave only %d interleavings", len(outputs))
		}
	})
}

// serverChild is a recorded server.
type serverChild struct {
	cmd  *exec.Cmd
	out  *bufio.Reader
	errb *bytes.Buffer
	port int
}

func startServer(t *testing.T, bin, dir string, env ...string) *serverChild {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), env...), fmt.Sprintf("PORT=%d", port))
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &serverChild{cmd: cmd, out: bufio.NewReader(pipe), errb: &errb, port: port}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	line, err := c.out.ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("server start: %q %v\n%s", line, err, errb.String())
	}
	return c
}

func (c *serverChild) get(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", c.port, path), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// stop sends SIGINT and returns the exit status and the rest of stdout.
func (c *serverChild) stop(t *testing.T) (int, string) {
	t.Helper()
	if err := c.cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(c.out)
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), string(rest)
		}
		return 0, string(rest)
	case <-time.After(20 * time.Second):
		t.Fatalf("the server did not stop:\n%s", c.errb.String())
	}
	return 0, ""
}

// TestReplayServer records an HTTP server that takes requests from the
// network, including one whose handler faults (isolated: 500 and a crash
// bundle), and replays it offline: the requests come from the trace, the
// output and the responses are the same. The bundle replays to the same
// isolated fault, also from ring buffers (XO_RING) alone.
func TestReplayServer(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	replayBuilds(t, func(t *testing.T, race bool) {
		bin := buildReplayProg(t, filepath.Join("testdata", "replay", "server"), race)
		dir := t.TempDir()
		crash := t.TempDir()
		trace := filepath.Join(dir, "server.xotrace")
		c := startServer(t, bin, dir, "XO_RECORD="+trace, "XO_CRASH_DIR="+crash)
		if st, body := c.get(t, "GET", "/hello/ann", ""); st != 200 || !strings.HasPrefix(body, "hello ann ") {
			t.Fatalf("hello: %d %q", st, body)
		}
		if st, body := c.get(t, "POST", "/echo", "payload"); st != 200 || body != "payload" {
			t.Fatalf("echo: %d %q", st, body)
		}
		// arena backed bodies (decision 0066): the second, shorter
		// request reuses the arena of the first one; the trace must keep
		// both bodies and both responses as they were
		items := []string{`{"name":"first-and-much-longer"}`, `{"name":"b"}`}
		for _, it := range items {
			if st, body := c.get(t, "POST", "/item", it); st != 200 || body != it {
				t.Fatalf("item: %d %q", st, body)
			}
		}
		if st, _ := c.get(t, "GET", "/boom", ""); st != 500 {
			t.Fatalf("boom: %d", st)
		}
		if st, _ := c.get(t, "GET", "/hello/bob", ""); st != 200 {
			t.Fatalf("hello bob: %d", st)
		}
		code, rest := c.stop(t)
		recdOut := "ready\n" + rest
		if code != 0 || !strings.HasSuffix(recdOut, "served 3\n") {
			t.Fatalf("recording: exit %d\n%s\n%s", code, recdOut, c.errb.String())
		}
		recdErr := c.errb.String()
		data, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"req":1`, `"req":6`, `"path":"/boom"`, `"status":500`, ` r1 `, ` g `, `first-and-much-longer`, `"body":"{\"name\":\"b\"}"`} {
			if !bytes.Contains(data, []byte(want)) {
				t.Errorf("the trace has no %s:\n%s", want, data)
			}
		}

		// offline: the port may be taken now; the replay does not listen
		rep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+trace)
		if rep.code != 0 || rep.stdout != recdOut || programStderr(rep.stderr) != programStderr(recdErr) {
			t.Fatalf("replay differs: exit %d\n--- stdout\n%s--- recorded\n%s--- stderr\n%s--- recorded\n%s", rep.code, rep.stdout, recdOut, rep.stderr, recdErr)
		}
		if !strings.Contains(rep.stderr, "xo replay: identical run: exit status 0") {
			t.Errorf("verdict:\n%s", rep.stderr)
		}

		// the bundle of the isolated fault
		bundle := bundlePath(t, recdErr)
		brep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+bundle)
		if brep.code != 0 || !strings.Contains(brep.stderr, "xo replay: reproduced the crash: fault: index 5 out of range for List of length 1 at main.xo:35:23 (isolated, in the handler of request 5)") {
			t.Fatalf("bundle replay: exit %d\n%s\n%s", brep.code, brep.stdout, brep.stderr)
		}

		// ring buffers only: main's startup and the failing request
		ringCrash := t.TempDir()
		r := startServer(t, bin, dir, "XO_RING=64", "XO_CRASH_DIR="+ringCrash)
		r.get(t, "GET", "/hello/cy", "")
		if st, _ := r.get(t, "GET", "/boom", ""); st != 500 {
			t.Fatalf("boom: %d", st)
		}
		if code, _ := r.stop(t); code != 0 {
			t.Fatalf("ring run: exit %d\n%s", code, r.errb.String())
		}
		ringBundle := bundlePath(t, r.errb.String())
		rrep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+ringBundle)
		if rrep.code != 0 || !strings.Contains(rrep.stderr, "xo replay: reproduced the crash: fault: index 5 out of range") {
			t.Fatalf("ring bundle replay: exit %d\n%s\n%s", rrep.code, rrep.stdout, rrep.stderr)
		}
	})
}

// TestReplayDivergence replays a trace with a changed program: the first
// effect it asks for differently is reported with both locations.
func TestReplayDivergence(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	src, err := os.ReadFile(filepath.Join("testdata", "replay", "effects", "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	prog := filepath.Join(dir, "effects")
	os.MkdirAll(prog, 0o755)
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	os.WriteFile(filepath.Join(work, "data.txt"), []byte("x\n"), 0o644)
	trace := filepath.Join(dir, "run.xotrace")
	bin := buildReplayProg(t, prog, false)
	recd := runProg(t, bin, work, "abc\n", "XO_RECORD="+trace, "XO_CRASH_DIR="+t.TempDir())
	if recd.code != 70 {
		t.Fatalf("recording: exit %d\n%s", recd.code, recd.stderr)
	}
	changed := strings.Replace(string(src), `"data.txt"`, `"other.txt"`, 1)
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	bin = buildReplayProg(t, prog, false)
	rep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+trace)
	want := "effect 6 of main differs:\n  the program calls  fs.read(\"other.txt\") at main.xo:14\n  the trace recorded fs.read(\"data.txt\") at main.xo:14\n"
	if rep.code != 3 || !strings.Contains(rep.stderr, want) {
		t.Fatalf("divergence: exit %d\n%s\nwant:\n%s", rep.code, rep.stderr, want)
	}
	// the output up to the divergence is the recorded one
	if !strings.HasPrefix(recd.stdout, rep.stdout) || !strings.Contains(rep.stdout, "line: abc") {
		t.Errorf("replayed output %q is not a prefix of %q", rep.stdout, recd.stdout)
	}
}
