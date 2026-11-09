package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRepl: the experimental `xo repl` (decision 0572 stage 1) with
// --eval and --session. Values print, definitions and std calls work,
// a plain binding carries to later entries and later calls as a pinned
// literal (its entry's effect does not run again), a var keeps its latest
// value, rebinding replaces, an entry that does not check is reported at
// the entry's own lines and leaves the session as it was, and the command
// is off without XO_REPL=1.
func TestRepl(t *testing.T) {
	t.Parallel()
	xo(t, "repl", "--eval=1 + 2").want(2)
	setenv(t, "XO_REPL", "1")

	r := xo(t, "repl", "--eval="+strings.Join([]string{
		`1 + 2`,
		`fn sq(n: Int) -> Int {`,
		`  n * n`,
		`}`,
		`sq(7)`,
		`let words = "a b c".split(" ")`,
		`os.stdio.println("once")`,
		`words.len()`,
		`var n = 1`,
		`n += 41`,
		`n`,
		`let n2 = "${n}!"`,
		`n2`,
	}, "\n")).want(0)
	want := "3\n49\nonce\n3\n42\n\"42!\"\n"
	if r.out != want {
		t.Errorf("--eval output:\n%s\nwant:\n%s\nstderr:\n%s", r.out, want, r.err)
	}

	// a session file carries the bindings to the next call; a printing
	// entry of the first call does not print again
	sess := filepath.Join(t.TempDir(), "s.json")
	xo(t, "repl", "--session="+sess, "--eval="+strings.Join([]string{
		`let greeting = "hi"`,
		`os.stdio.println("first call")`,
		`struct P { x: Int, y: Int }`,
		`let p = P{x: 1, y: 2}`,
	}, "\n")).want(0)
	if _, err := os.Stat(sess); err != nil {
		t.Fatalf("no session file: %v", err)
	}
	r = xo(t, "repl", "--session="+sess, "--eval="+strings.Join([]string{
		`greeting + "!"`,
		`p.x + p.y`,
		`let greeting = 5`,
		`greeting * 2`,
	}, "\n")).want(0)
	want = "\"hi!\"\n3\n10\n"
	if r.out != want {
		t.Errorf("second call:\n%s\nwant:\n%s\nstderr:\n%s", r.out, want, r.err)
	}

	// definitions again (decision 0572 stage 2): a fn replaces the
	// earlier one; a struct's binding runs its entry again with the new
	// struct, and one whose entry no longer checks is dropped
	r = xo(t, "repl", "--eval="+strings.Join([]string{
		`fn sq(x: Int) -> Int { x + x }`,
		`sq(5)`,
		`fn sq(x: Int) -> Int { x * x }`,
		`sq(5)`,
		`struct P { x: Int }`,
		`let p = P{x: 1}`,
		`let q = P{x: 2}`,
		`fn mk() -> P { P{x: 3} }`,
		`let m = mk()`,
		`struct P { x: Int, y: Int }`,
		`fn mk() -> P { P{x: 3, y: 4} }`,
		`let p = P{x: 1, y: 2}`,
		`p.x + p.y`,
	}, "\n")).want(0)
	if want := "10\n25\n3\n"; r.out != want || !strings.Contains(r.err, "dropped `p`") || !strings.Contains(r.err, "dropped `q`") || !strings.Contains(r.err, "definition of `mk`") {
		t.Errorf("redefinitions:\n%s\nwant:\n%s\nstderr:\n%s", r.out, want, r.err)
	}

	// :defs, :save and :load (decision 0572 stage 2)
	saved := filepath.Join(t.TempDir(), "saved.xo")
	r = xo(t, "repl", "--eval="+strings.Join([]string{
		`use std/adler32`,
		`fn sq(x: Int) -> Int {`,
		`  x * x`,
		`}`,
		`let n = sq(3)`,
		`adler32.checksum(b"a")`,
		`:defs`,
		`:save ` + saved,
	}, "\n")).want(0)
	if want := "use std/adler32\nfn sq(x: Int) -> Int {\n  x * x\n}\nlet n: Int\n"; !strings.HasSuffix(r.out, want) {
		t.Errorf(":defs:\n%s\nwant:\n%s\nstderr:\n%s", r.out, want, r.err)
	}
	r = xo(t, "repl", "--eval=:load "+saved+"\nsq(n)").want(0)
	if r.out != "81\n" || !strings.Contains(r.err, "loaded 3 entries") {
		t.Errorf(":load: %q, stderr %q", r.out, r.err)
	}

	// an entry that does not check: the diagnostic names the entry's line
	r = xo(t, "repl", "--eval="+strings.Join([]string{
		`let a = 1`,
		`a + "x"`,
	}, "\n")).want(1)
	if !strings.Contains(r.err, "entry:1:") || !strings.Contains(r.err, "error XO") {
		t.Errorf("a type error: want a diagnostic at entry:1, got:\n%s", r.err)
	}
}

// TestReplWarmup: an interactive `xo repl` starts a warm-up build of a
// session program while the person types (decision 0572 stage 2), waits
// for it before the first entry's build, and removes its directory and
// the session's at the end, also when the session ends before any entry.
func TestReplWarmup(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"1 + 2\n:quit\n", ":quit\n"} {
		tmp := t.TempDir()
		cmd := exec.Command(xoBin(t), "repl")
		cmd.Env = append(os.Environ(), "XO_REPL=1", "XO_REPL_TIMES=1", "TMPDIR="+tmp)
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("xo repl with %q: %v", input, err)
		}
		if strings.HasPrefix(input, "1 + 2") && string(out) != "3\n" {
			t.Errorf("xo repl with %q: output %q, want \"3\\n\"", input, out)
		}
		left, _ := filepath.Glob(filepath.Join(tmp, "xo-repl-*"))
		if len(left) != 0 {
			t.Errorf("xo repl with %q left %v", input, left)
		}
	}
}

// TestReplLineEditing: on a terminal, `xo repl` edits entries with the
// line editor on raw input (decision 0572 stage 2 step 1, decision 0581),
// under script(1): a backspace edits an entry, an open brace continues
// it on a new line, up recalls the last entry for editing, ctrl-d ends
// the session, and the history file keeps the entries.
func TestReplLineEditing(t *testing.T) {
	if testing.Short() {
		t.Skip("runs builds")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("script(1) on darwin and linux")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("no script(1) for a pseudo terminal")
	}
	t.Parallel()
	cfg := t.TempDir()
	x := xoBin(t)
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.Command("script", "-q", "/dev/null", x, "repl")
	} else {
		c = exec.Command("script", "-qec", x+" repl", "/dev/null")
	}
	c.Env = append(os.Environ(), "XO_REPL=1", "XDG_CONFIG_HOME="+cfg, "HOME="+cfg)
	in, err := c.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	c.Stdout = &out
	c.Stderr = &out
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	// each key group after the REPL has set raw input and run the entry
	// before (bytes the terminal gets earlier are read cooked)
	for _, k := range []string{"1 + 3\x7f2\r", "fn sq(x: Int) -> Int {\r", "  x * x\r}\r", "sq(5)\r", "\x1b[A\x7f\x7f6)\r", "\x04"} {
		time.Sleep(4 * time.Second)
		in.Write([]byte(k))
	}
	time.Sleep(2 * time.Second)
	in.Close()
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		c.Process.Kill()
		t.Fatalf("the session did not end: %q", out.String())
	}
	text := regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]").ReplaceAllString(strings.ReplaceAll(out.String(), "\r", ""), "")
	for _, want := range []string{"\n3\n", "\n25\n", "\n36\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("the session's output has no %q: %q", want, text)
		}
	}
	var hist []byte
	filepath.WalkDir(cfg, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Name() == "repl_history" {
			hist, _ = os.ReadFile(p)
		}
		return nil
	})
	if got := strings.Split(string(hist), "\x00"); len(got) != 4 || got[1] != "fn sq(x: Int) -> Int {\n  x * x\n}" || got[3] != "sq(6)" {
		t.Errorf("history: %q", got)
	}
}
