package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestCMarkLayout: the C adapters of `use c` (compiler/cli/bind gen.xo,
// decision 0675, #107) mark a C call inline through the worker record in
// xo_tlocal slot CSEQ_TLS_W and its word CSEQ_WORD, copies of
// std/native/llrt's sched.xo TLS_W and layout.xo W_CSEQ_I, which a move of
// either would leave writing a stale word.
func TestCMarkLayout(t *testing.T) {
	val := func(rel, name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`(?m)^\s*(?:pub\s+)?const\s+` + name + `\s*(?::\s*\w+\s*)?=\s*(\d+)`).FindSubmatch(b)
		if m == nil {
			t.Fatalf("%s: no const %s", rel, name)
		}
		return string(m[1])
	}
	gen := filepath.Join("compiler", "cli", "bind", "gen.xo")
	for _, c := range []struct{ copy, rel, name string }{
		{"CSEQ_TLS_W", filepath.Join("std", "native", "llrt", "sched.xo"), "TLS_W"},
		{"CSEQ_WORD", filepath.Join("std", "native", "llrt", "layout.xo"), "W_CSEQ_I"},
	} {
		if got, want := val(gen, c.copy), val(c.rel, c.name); got != want {
			t.Errorf("%s %s = %s, but %s %s = %s", gen, c.copy, got, c.rel, c.name, want)
		}
	}
}
