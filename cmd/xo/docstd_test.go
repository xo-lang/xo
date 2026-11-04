package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDocStdList: `xo doc std` lists the std modules this xo ships, a
// line each with its summary (the AWS agent's feedback log, 2026-11-04:
// "does std have X" had no one command answer).
func TestDocStdList(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"doc", "std"}, &out, &errOut); code != 0 {
		t.Fatalf("xo doc std: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	got := out.String()
	for _, want := range []string{"std/jwt ", "JSON Web Tokens", "std/http ", "see spec/std.md 4", "modules in "} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "std/prelude") || strings.Contains(got, "Declaration file") {
		t.Errorf("the prelude or a boilerplate summary is listed:\n%s", got)
	}
}
