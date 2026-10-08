package main

import (
	"bufio"
	"bytes"
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

var dumpHeader = regexp.MustCompile(`^xo dump: pid \d+, 3 tasks\n`)

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
	if testing.Short() {
		t.Skip("builds Go")
	}
	if runtime.GOOS == "windows" {
		t.Skip("no SIGQUIT on windows")
	}
	dir := t.TempDir()
	t.Setenv("XO_BUILD_DIR", filepath.Join(dir, "build"))
	t.Setenv("XO_DUMP_DIR", dir)
	src := filepath.Join(dir, "app")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(dumpProg), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	bin := filepath.Join(dir, "app.bin")
	if code := run([]string{"build", "-o", bin, src}, &out, &errOut); code != 0 {
		t.Fatalf("build: exit %d: %s", code, errOut.String())
	}
	cmd := exec.Command(bin)
	cmd.Dir = src
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
		if code := run([]string{"dump", pid}, &out, &errOut); code != 0 {
			t.Fatalf("xo dump: exit %d: %s", code, errOut.String())
		}
		got = out.String()
		if strings.HasSuffix(got, wantDump) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !dumpHeader.MatchString(got) || !strings.HasSuffix(got, wantDump) || len(dumpHeader.FindString(got))+len(wantDump) != len(got) {
		t.Fatalf("dump:\n%s\nwant (after the header):\n%s", got, wantDump)
	}
	// the program printed the same dump on its standard error and goes on
	if !strings.Contains(stderr.String(), wantDump) {
		t.Errorf("stderr lacks the dump:\n%s", stderr.String())
	}
	if cmd.ProcessState != nil {
		t.Errorf("the program exited after the dump")
	}
	if _, err := os.Stat(dumpPath(cmd.Process.Pid)); err == nil {
		t.Errorf("xo dump left %s behind", dumpPath(cmd.Process.Pid))
	}
}

func TestDumpUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"dump"}, &out, &errOut); code != 2 {
		t.Errorf("no pid: exit %d", code)
	}
	if code := run([]string{"dump", "abc"}, &out, &errOut); code != 2 {
		t.Errorf("bad pid: exit %d", code)
	}
}
