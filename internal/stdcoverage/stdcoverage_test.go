package stdcoverage

import "testing"

// TestCallsIn: the scanner finds a bare call, a qualified call, one with
// type arguments, and a call after a field access.
func TestCallsIn(t *testing.T) {
	calls := callsIn(`let x = s.contains("a") + y.find_where[E](f) + Int.parse(s) + t.await`)
	for _, want := range []string{"contains", "s.contains", "find_where", "y.find_where", "parse", "Int.parse"} {
		if !calls[want] {
			t.Errorf("callsIn missed %q: %v", want, calls)
		}
	}
	if calls["await"] {
		t.Errorf("callsIn treated t.await as a call")
	}
}

// TestRuleMatch: exact symbols and families, and nothing else.
func TestRuleMatch(t *testing.T) {
	exact := Rule{Pattern: "prelude.Cmd.inherit"}
	if !exact.Match(Entry{Module: "prelude", Symbol: "Cmd.inherit"}) {
		t.Error("exact rule did not match")
	}
	if exact.Match(Entry{Module: "prelude", Symbol: "Cmd.inherit2"}) {
		t.Error("exact rule matched another symbol")
	}
	family := Rule{Pattern: "prelude.llrt_*"}
	if !family.Match(Entry{Module: "prelude", Symbol: "llrt_print"}) {
		t.Error("family rule did not match")
	}
	if family.Match(Entry{Module: "debug", Symbol: "llrt_print"}) {
		t.Error("family rule matched another module")
	}
}

// TestParseAllow: a reason is required, so an allowlist line cannot be a
// bare symbol.
func TestParseAllow(t *testing.T) {
	rules, err := ParseAllow("# comment\n\nprelude.Cmd.inherit  needs a real child\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Reason != "needs a real child" {
		t.Fatalf("parsed %+v", rules)
	}
	if _, err := ParseAllow("prelude.Cmd.inherit\n"); err == nil {
		t.Fatal("a line with no reason was accepted")
	}
}

// TestAllowParses keeps the embedded file well formed.
func TestAllowParses(t *testing.T) {
	rules, err := Allow()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 5 {
		t.Fatalf("allow.txt has %d rules, too few for the internals it covers", len(rules))
	}
}
