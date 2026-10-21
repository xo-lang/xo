package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDevScaffolds covers xo dev new-code and new-golden (2026-10-20 poll)
// in a fake checkout: the code's rows land in code order in both tables
// with an errorcheck case, a repeat is refused, and a golden gets its
// program and the want.txt of a run.
func TestDevScaffolds(t *testing.T) {
	root := writeFiles(t, map[string]string{
		"spec/core.md":                      "# Spec\n\n### 10.6 Diagnostic codes\n\n| Code | Severity | Reported by | Meaning |\n|---|---|---|---|\n| `XO0101` | error | checker | a |\n| `XO0160` | error | parser | b |\n\n### 10.7 Next\n\n| `XO0001` | x | y | z |\n",
		"compiler/cli/core/explain_data.xo": "pub fn explanation(code: Str) -> Expl? {\n  match code {\n    \"XO0101\" => Some(Expl{title: \"a\"})\n    \"XO0160\" => Some(Expl{title: \"b\"})\n    _ => None\n  }\n}\n",
	})
	chdir(t, root)
	xo := func(code int, args ...string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		if got := run(t, args, &out, &errOut); got != code {
			t.Fatalf("xo %v: exit %d, want %d: %s%s", args, got, code, out.String(), errOut.String())
		}
		return out.String()
	}
	xo(0, "dev", "new-code", "XO0150", "--title=a \"quoted\" title", "--component=parser", "--ref=core 1.3")
	spec, _ := os.ReadFile(filepath.Join(root, "spec/core.md"))
	if !strings.Contains(string(spec), "| `XO0101` | error | checker | a |\n| `XO0150` | error | parser | a \"quoted\" title |\n| `XO0160`") {
		t.Errorf("spec:\n%s", spec)
	}
	if strings.Count(string(spec), "XO0150") != 1 {
		t.Errorf("XO0150 written outside 10.6:\n%s", spec)
	}
	ex, _ := os.ReadFile(filepath.Join(root, "compiler/cli/core/explain_data.xo"))
	if !strings.Contains(string(ex), "\"XO0101\" => Some(Expl{title: \"a\"})\n    \"XO0150\" => Some(Expl{title: \"a \\\"quoted\\\" title\", severity: \"error\", summary: \"a \\\"quoted\\\" title (core 1.3)\"") {
		t.Errorf("explain_data.xo:\n%s", ex)
	}
	if b, err := os.ReadFile(filepath.Join(root, "test/errorcheck/xo0150/main.xo")); err != nil || !strings.Contains(string(b), "// ERROR XO0150") {
		t.Errorf("errorcheck case: %v\n%s", err, b)
	}
	xo(1, "dev", "new-code", "XO0150", "--title=again")
	xo(2, "dev", "new-code", "XO15", "--title=bad")

	xo(0, "dev", "new-golden", "--dir=golden", "hello_scaffold")
	want, err := os.ReadFile(filepath.Join(root, "golden/hello_scaffold/want.txt"))
	if err != nil || string(want) != "exit 0\n--- stdout\nhello_scaffold\n--- stderr\n" {
		t.Errorf("want.txt = %q, %v", want, err)
	}
	xo(1, "dev", "new-golden", "--dir=golden", "hello_scaffold")
}
