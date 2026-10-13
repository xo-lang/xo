package diag

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestScriptsReadWantBytes keeps the golden harness scripts reading
// want.txt without newline translation. Python's text mode open turns a
// carriage return into "\n", so a golden whose output holds a raw "\r"
// (show_table prints '\r') never matched the program's output, and
// scripts/selfhost-goldens.py reported it as an xo1 failure (172 of 173,
// 2026-10-13; gotcha python-text-mode-want-cr.md). It lives here because
// diag already reads the repo's files (spec_test.go).
func TestScriptsReadWantBytes(t *testing.T) {
	files, err := filepath.Glob("../../scripts/*.py")
	if err != nil {
		t.Fatal(err)
	}
	read := regexp.MustCompile(`open\([^\n]*'want\.txt'\)[^\n]*`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := read.FindString(line)
			if m != "" && !strings.Contains(m, "newline=''") && !strings.Contains(m, "'rb'") {
				t.Errorf("%s: %q reads want.txt in text mode, which turns \\r into \\n; pass newline='' (or 'rb')", filepath.Base(f), strings.TrimSpace(line))
			}
		}
	}
}
