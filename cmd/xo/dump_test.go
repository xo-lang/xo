package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// dumpProg holds a mutex in one task (sleeping for an hour) while a second
// task waits for it, and main waits at the end of the scope.
const dumpProg = `fn main(os: Os) {
  let m = Mutex[Int].new(0)
  let held = Chan[Int].new(cap: 1)
  os.stdio.println("ready")
  scope s {
    s.spawn(fn() {
      m.with(fn(var v) {
        held.send(1)
        os.clock.sleep(1h)
        v += 1
      })
    })
    s.spawn(fn() {
      let _ = held.recv()
      m.with(fn(v) { v })
    })
    // tasks that end at once stay in the scope's list until a later spawn
    // prunes them: the dump lists live tasks only
    for _ in 0..3 {
      s.spawn(fn() { })
    }
  }
}
`

// wantDump is the stable part of the dump: everything but the pid.
const wantDump = `main: blocked in scope end at main.xo:5 for task 2, task 3
  scope at main.xo:5: waiting for its tasks
    task 2 (spawned at main.xo:6): blocked in sleep at main.xo:9
    task 3 (spawned at main.xo:13): blocked in Mutex.with at main.xo:15 on a mutex made at main.xo:2, held by task 2 since main.xo:7
end of xo dump
`

// wantDumpNative is the native runtime's (decision 0251): the same tree
// and waits in the Go backend's words, without the Go backend's scope
// sites, main's own wait, and the mutex's holder (follow ups).
const wantDumpNative = `main: in scope
  scope: waiting for its tasks
    task 2 (spawned at main.xo:6): blocked in sleep at main.xo:9
    task 3 (spawned at main.xo:13): blocked in Mutex.with at main.xo:15
end of xo dump
`

var dumpHeader = regexp.MustCompile(`^xo dump: pid \d+, 3 tasks\n`)

// dumpPath is the file a program writes its dump to for `xo dump`:
// $XO_DUMP_DIR (else the temporary directory)/xo-dump-<pid>.txt.
func dumpPath(t testing.TB, pid int) string {
	dir := getenv(t, "XO_DUMP_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "xo-dump-"+strconv.Itoa(pid)+".txt")
}

// lockedBuffer is a bytes.Buffer safe for the copying goroutine of exec.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestDump runs `xo dump <pid>` against a program blocked in known places
// (core 11.4): the program prints the task tree on SIGQUIT, writes it for
// `xo dump`, and goes on running. Only this test's own child is signaled.
func TestDump(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no SIGQUIT on windows")
	}
	t.Run("go", func(t *testing.T) { testDump(t, "--backend=go", nil, wantDump) })
	// the native runtime's dump (decision 0251): the poller's thread writes
	// it, so it comes with one worker (XO_THREADS=1) too
	native := "--backend=" + xotool.NativeBackend()
	t.Run("native", func(t *testing.T) { testDump(t, native, nil, wantDumpNative) })
	t.Run("native1", func(t *testing.T) { testDump(t, native, []string{"XO_THREADS=1"}, wantDumpNative) })
}

// testDump builds dumpProg with backend, runs it with env, asks it for
// its dump with `xo dump`, and checks the dump after its header is want.
func testDump(t *testing.T, backend string, env []string, want string) {
	dir := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(dir, "build"))
	setenv(t, "XO_DUMP_DIR", dir)
	src := filepath.Join(dir, "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(dumpProg), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	bin := filepath.Join(dir, "app.bin")
	if code := run(t, []string{"build", backend, "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	cmd := command(t, bin)
	cmd.Dir = src
	if len(env) > 0 {
		base := cmd.Env
		if base == nil {
			base = os.Environ()
		}
		cmd.Env = append(base, env...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill() // our own child, by its PID
		cmd.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("first line %q, %v", line, err)
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	// the tasks reach their waits soon after "ready": ask until they did
	var got string
	for i := 0; i < 50; i++ {
		out.Reset()
		errOut.Reset()
		if code := run(t, []string{"dump", pid}, &out, &errOut); code != 0 {
			t.Fatalf("xo dump: exit %d: %s", code, errOut.String())
		}
		got = out.String()
		if strings.HasSuffix(got, want) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !dumpHeader.MatchString(got) || !strings.HasSuffix(got, want) || len(dumpHeader.FindString(got))+len(want) != len(got) {
		t.Fatalf("dump:\n%s\nwant (after the header):\n%s", got, want)
	}
	// the program printed the same dump on its standard error and goes on
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr lacks the dump:\n%s", stderr.String())
	}
	if cmd.ProcessState != nil {
		t.Errorf("the program exited after the dump")
	}
	if _, err := os.Stat(dumpPath(t, cmd.Process.Pid)); err == nil {
		t.Errorf("xo dump left %s behind", dumpPath(t, cmd.Process.Pid))
	}
}

func TestDumpUsage(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"dump"}, &out, &errOut); code != 2 {
		t.Errorf("no pid: exit %d", code)
	}
	if code := run(t, []string{"dump", "abc"}, &out, &errOut); code != 2 {
		t.Errorf("bad pid: exit %d", code)
	}
}
