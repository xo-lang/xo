package main

import (
	"bytes"
	"encoding/json"
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
	for _, want := range []string{"std/jwt ", "JSON Web Tokens", "std/http ", "HTTP servers and clients", "modules in "} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "std/prelude") || strings.Contains(got, "Declaration file") {
		t.Errorf("the prelude or a boilerplate summary is listed:\n%s", got)
	}
}

// TestDocStdSummaries: every std module's summary is the whole first
// sentence of what its header says the module is for: none empty, cut
// ("..."), boilerplate ("Pure.", "Notes for this file"), a reference
// alone ("see spec/std.md 4") or a decision citation (pkg.xolang.org and
// xolang.dev show these lines).
func TestDocStdSummaries(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	if code := run(t, []string{"doc", "--json", "std"}, &out, &errOut); code != 0 {
		t.Fatalf("xo doc --json std: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	var rows []struct{ Module, Summary string }
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("xo doc --json std: %v\n%s", err, out.String())
	}
	if len(rows) < 50 {
		t.Fatalf("only %d modules", len(rows))
	}
	for _, r := range rows {
		s := r.Summary
		bad := s == "" || strings.HasSuffix(s, "...") || strings.HasPrefix(s, "see ") ||
			strings.Contains(s, "Pure.") || strings.Contains(s, "Notes for this file") ||
			strings.Contains(s, "Declaration file") || strings.Contains(s, "(decision") ||
			strings.HasPrefix(s, "std/")
		if bad {
			t.Errorf("%s: summary %q", r.Module, s)
		}
	}
}
