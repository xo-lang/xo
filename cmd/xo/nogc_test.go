package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// nativeBackends lists the native backends this host can build tests
// with.
func nativeBackends(t *testing.T) []string {
	t.Helper()
	var out []string
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		if _, err := build.CC(); err == nil {
			out = append(out, build.BackendArm64)
		}
	}
	if _, err := build.Clang(); err == nil {
		out = append(out, build.BackendLLVM)
	}
	if len(out) == 0 {
		t.Skip("no native backend on this host")
	}
	return out
}

func xoTest(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"test"}, args...), &out, &errOut)
	return code, out.String() + errOut.String()
}

// TestNoGCTests runs native test binaries (`xo test --no-gc`, decision
// 0083): the report of testdata/nogc/same and of examples/notes-service
// is the Go backend's, line for line; testdata/nogc/native (error
// traces, fault frames, and deadlock details, which native builds do not
// record) matches its want.txt.
func TestNoGCTests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries on every backend")
	}
	backends := nativeBackends(t)
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("..", "..", "examples", "notes-service")} {
		goCode, goOut := xoTest(t, "-v", dir)
		for _, be := range backends {
			code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
			if code != goCode || out != goOut {
				t.Errorf("%s on %s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", dir, be, goCode, code, goOut, be, out)
			}
		}
	}
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
}

var leakLine = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// TestNoGCTestLeaks runs every passing test of the native test binaries
// with XO_LEAKCHECK=1: no allocation may be live at exit.
func TestNoGCTestLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	backends := nativeBackends(t)
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("..", "..", "examples", "notes-service")} {
		for _, be := range backends {
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Test, NoGC: true, Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("%s on %s: %v %v", dir, be, err, res.Diagnostics)
			}
			bin := res.Binaries[build.Host()]
			for _, ti := range res.Tests {
				cmd := exec.Command(bin)
				cmd.Dir = ti.Dir
				cmd.Env = append(os.Environ(), "XO_TEST="+strconv.Itoa(ti.Index), "XO_LEAKCHECK=1")
				var errb bytes.Buffer
				cmd.Stdout, cmd.Stderr = io.Discard, &errb
				if cmd.Run() != nil {
					continue // a failing test exits without the check
				}
				m := leakLine.FindStringSubmatch(errb.String())
				if m == nil {
					t.Errorf("%s %q on %s: no leak check line:\n%s", dir, ti.Name, be, errb.String())
					continue
				}
				if m[1] != "0" {
					t.Errorf("%s %q on %s: %s allocations live at exit (of %s)", dir, ti.Name, be, m[1], m[2])
				}
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
	if testing.Short() {
		t.Skip("builds and runs the service")
	}
	backends := append([]string{build.BackendGo}, nativeBackends(t)...)
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	dir := filepath.Join("..", "..", "examples", "notes-service")
	var want string
	for _, be := range backends {
		opts := build.Options{Mode: gen.Build, Backend: be, NoGC: be != build.BackendGo, Out: filepath.Join(t.TempDir(), "notes")}
		res, err := build.Build([]string{dir}, opts)
		if err != nil || res.Failed {
			t.Fatalf("%s: %v %v", be, err, res.Diagnostics)
		}
		cmd := exec.Command(res.Binaries[build.Host()])
		cmd.Env = append(os.Environ(), "NOTES_TOKEN=t0p", "NOTES_PASSWORD=correct horse", "NOTES_ADDR=127.0.0.1:0")
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
		if be == build.BackendGo {
			want = got
		} else if got != want {
			t.Errorf("%s answers differ from the Go backend's\n--- go\n%s--- %s\n%s", be, want, be, got)
		}
	}
}
