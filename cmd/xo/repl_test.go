package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	// an entry that does not check: the diagnostic names the entry's line
	r = xo(t, "repl", "--eval="+strings.Join([]string{
		`let a = 1`,
		`a + "x"`,
	}, "\n")).want(1)
	if !strings.Contains(r.err, "entry:1:") || !strings.Contains(r.err, "error XO") {
		t.Errorf("a type error: want a diagnostic at entry:1, got:\n%s", r.err)
	}
}
