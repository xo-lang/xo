package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrintHabitSaysWhatToWrite: `println` in a `fn main()` (or a function
// with no capability) is XO0601 with a note that names what to write
// (decision 0321 follow-up, poll item c of 2026-10-21).
func TestPrintHabitSaysWhatToWrite(t *testing.T) {
	cases := []struct{ src, want string }{
		{"fn main() {\n  println(\"hi\")\n}\n", "fn main(os: Os) { os.stdio.println(...) }"},
		{"fn show() {\n  println(\"hi\")\n}\n\nfn main(os: Os) {\n  show()\n}\n", "give `show` a `stdio: Stdio` parameter"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(c.src), 0o644); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)
		var out, errOut bytes.Buffer
		run(t, []string{"check", "."}, &out, &errOut)
		got := out.String() + errOut.String()
		if !strings.Contains(got, "XO0601") || !strings.Contains(got, c.want) {
			t.Errorf("for %q: want XO0601 and %q in:\n%s", c.src, c.want, got)
		}
	}
}
