package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestQueryOutsideModule (decision 0437): `xo query` on a directory with
// no xo.mod above it and no .xo files exits 2 and says to pass --dir,
// instead of an empty answer that reads as "none" (Lang 2, 2026-10-30).
func TestQueryOutsideModule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"query", "def", "main", "--dir=" + dir}, {"query", "callers", "main", "--dir=" + dir}} {
		var out, errOut bytes.Buffer
		if code := run(t, args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "not inside a module") || !strings.Contains(errOut.String(), "--dir") {
			t.Errorf("%v: want exit 2 naming --dir, got %d:\n%s%s", args, code, out.String(), errOut.String())
		}
	}
}
