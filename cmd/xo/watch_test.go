package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// watchProg prints its version, then runs until a stop request (SIGTERM
// from the watcher's handoff, core 7.5).
const watchProg = `use std/task

fn main(os: Os) {
  os.stdio.println("VERSION")
  while !task.stop_requested() {
    os.clock.sleep(20ms)
  }
}
`

// cycle is one line of `xo run --watch --json` (decision 0267).
type cycle struct {
	Cycle     int      `json:"cycle"`
	Changed   []string `json:"changed"`
	Build     string   `json:"build"`
	Output    string   `json:"output"`
	Restarted bool     `json:"restarted"`
	PID       int      `json:"pid"`
	Exited    *int     `json:"exited"`
}

func alive(pid int) bool { return pid > 0 && syscall.Kill(pid, 0) == nil }

// TestRunWatch: xo run --watch starts the program, leaves it running
// after an edit that does not change the binary (a new test file),
// restarts it after one that does (SIGTERM, the new pid), keeps it after
// a failed build, and stops it when xo gets a stop request.
func TestRunWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the program four times")
	}
	dir := writeFiles(t, map[string]string{"main.xo": strings.ReplaceAll(watchProg, "VERSION", "v1")})
	cmd := command(t, xoBin(t), "run", "--watch", "--json", "--grace=5s", dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var pids []int
	ended := false
	defer func() {
		// only what this test started: xo (while it has not been waited
		// for, so its pid is still its own), then any program it left
		if !ended {
			cmd.Process.Kill()
			cmd.Wait()
		}
		for _, p := range pids {
			if alive(p) {
				t.Errorf("program %d still running after xo ended", p)
				syscall.Kill(p, syscall.SIGKILL)
			}
		}
	}()
	lines := make(chan cycle, 16)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var c cycle
			if json.Unmarshal(sc.Bytes(), &c) == nil && c.Cycle > 0 {
				lines <- c
			}
		}
		close(lines)
	}()
	next := func(what string) cycle {
		t.Helper()
		select {
		case c, ok := <-lines:
			if !ok {
				t.Fatalf("%s: xo run --watch ended", what)
			}
			t.Logf("%s: %+v", what, c)
			return c
		case <-time.After(3 * time.Minute):
			t.Fatalf("%s: no cycle", what)
		}
		return cycle{}
	}
	write := func(name, text string) {
		t.Helper()
		// past the modified time's millisecond and the debounce
		time.Sleep(50 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := next("first build")
	if c.Build != "rebuilt" || c.Restarted || !alive(c.PID) {
		t.Fatalf("first cycle %+v", c)
	}
	first := c.PID
	pids = append(pids, first)

	write("main_test.xo", "test \"t\" {\n  expect(1 == 1)\n}\n")
	c = next("a test file")
	if c.Build != "unchanged" || c.Restarted || !alive(first) {
		t.Errorf("a new test file: %+v (want unchanged, %d still running)", c, first)
	}

	write("main.xo", strings.ReplaceAll(watchProg, "VERSION", "v2"))
	c = next("an edit of main")
	if c.Build != "rebuilt" || !c.Restarted || c.PID == first || !alive(c.PID) {
		t.Fatalf("an edit of main: %+v", c)
	}
	pids = append(pids, c.PID)
	if alive(first) {
		t.Errorf("the first program %d still runs after the restart", first)
	}
	second := c.PID

	write("main.xo", "fn main( {\n")
	c = next("a broken edit")
	if c.Build != "failed" || c.Restarted || !alive(second) {
		t.Errorf("a broken edit: %+v (want failed, %d still running)", c, second)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		ended = true
	case <-time.After(20 * time.Second):
		t.Fatal("xo run --watch did not end after SIGTERM")
	}
	if alive(second) {
		t.Errorf("the program %d still runs after xo ended", second)
	}
	if _, err := os.Stat(filepath.Join(dir, ".xo", "watch", "last.json")); err != nil {
		t.Errorf("last.json: %v", err)
	}
}

// testCycle is one line of `xo test --watch --json` (decision 0267).
type testCycle struct {
	Cycle      int               `json:"cycle"`
	Ran        []string          `json:"ran"`
	Unaffected int               `json:"unaffected"`
	Status     int               `json:"status"`
	Results    []json.RawMessage `json:"results"`
}

// statuses are the results' test names by status.
func (c testCycle) statuses() map[string]string {
	out := map[string]string{}
	for _, r := range c.Results {
		var v struct{ Name, Status string }
		if json.Unmarshal(r, &v) == nil && v.Name != "" {
			out[v.Name] = v.Status
		}
	}
	return out
}

// TestTestWatch: xo test --watch runs every test first, then only the
// tests an edit reaches: the one test that names a changed function, a
// failing one after a broken edit, and a new test file's tests.
func TestTestWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the tests four times")
	}
	lib := "pub fn double(x: Int) -> Int { x * 2 }\n\npub fn triple(x: Int) -> Int { x * 3 }\n"
	dir := writeFiles(t, map[string]string{
		"lib.xo":    lib,
		"a_test.xo": "test \"double\" {\n  expect(double(2) == 4)\n}\n\ntest \"triple\" {\n  expect(triple(2) == 6)\n}\n",
	})
	cmd := command(t, xoBin(t), "test", "--watch", "--json", dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ended := false
	defer func() {
		if !ended {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	lines := make(chan testCycle, 16)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var c testCycle
			if json.Unmarshal(sc.Bytes(), &c) == nil && c.Cycle > 0 {
				lines <- c
			}
		}
		close(lines)
	}()
	next := func(what string) testCycle {
		t.Helper()
		select {
		case c, ok := <-lines:
			if !ok {
				t.Fatalf("%s: xo test --watch ended", what)
			}
			t.Logf("%s: ran %v, %d unaffected, status %d, %v", what, c.Ran, c.Unaffected, c.Status, c.statuses())
			return c
		case <-time.After(3 * time.Minute):
			t.Fatalf("%s: no cycle", what)
		}
		return testCycle{}
	}
	write := func(name, text string) {
		t.Helper()
		time.Sleep(50 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	c := next("first run")
	if st := c.statuses(); st["double"] != "pass" || st["triple"] != "pass" {
		t.Fatalf("first run: %v", st)
	}

	write("lib.xo", strings.Replace(lib, "x * 3", "x + x + x", 1))
	c = next("an edit of triple")
	if st := c.statuses(); len(st) != 1 || st["triple"] != "pass" || c.Unaffected != 1 {
		t.Errorf("an edit of triple ran %v (%d unaffected), want triple only", st, c.Unaffected)
	}

	write("lib.xo", strings.Replace(lib, "x * 2", "x * 5", 1))
	c = next("a broken double")
	if st := c.statuses(); st["double"] != "fail" || c.Status == 0 {
		t.Errorf("a broken double: %v, status %d", st, c.Status)
	}

	write("b_test.xo", "test \"quad\" {\n  expect(double(double(1)) == 20)\n}\n")
	c = next("a new test file")
	if st := c.statuses(); st["quad"] == "" || st["triple"] != "" {
		t.Errorf("a new test file ran %v, want quad and not triple", st)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		ended = true
	case <-time.After(20 * time.Second):
		t.Fatal("xo test --watch did not end after SIGTERM")
	}
}
