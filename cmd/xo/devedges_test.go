package main

import (
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestDevEdges: `xo dev edges` on one golden prints one tab separated line
// per resolved reference (SRC_PATH SRC_LINE SRC_NAME TGT_PATH TGT_LINE
// TGT_NAME SITE_PATH SITE_LINE SITE_COL SITE_NAME), and the method call the
// text names only by its receiver variable (`s.find_where(...)` in
// first_space) resolves to the std declaration Str.find_where, which is the
// reason the lookup goes through the checker and not a name match.
func TestDevEdges(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal", "llvm", "testdata", "golden", "strwhere")
	var out, errOut bytes.Buffer
	if code := run(t, []string{"dev", "edges", dir}, &out, &errOut); code != 0 {
		t.Fatalf("xo dev edges: exit %d: %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 100 {
		t.Fatalf("xo dev edges: %d lines, want the golden's references", len(lines))
	}
	found := false
	for i, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 10 {
			t.Fatalf("line %d: %d fields, want 10: %q", i+1, len(f), line)
		}
		if _, err := strconv.Atoi(f[4]); err != nil {
			t.Fatalf("line %d: target line %q: %v", i+1, f[4], err)
		}
		if f[3] == "std/prelude.xo" && f[4] == "271" && f[5] == "Str.find_where" && f[9] == "find_where" && f[2] == "first_space" {
			found = true
		}
	}
	if !found {
		t.Errorf("no edge from first_space's s.find_where() to std/prelude.xo:271 Str.find_where")
	}
}
