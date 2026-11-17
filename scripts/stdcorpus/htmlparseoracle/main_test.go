package main

import "testing"

// A random case x/net/html panics on has no answer to copy: it is dropped
// (#152, fuzz seed 33).
func TestKeepRandomDropsGoPanics(t *testing.T) {
	bad := &testCase{text: "</button></html></tr>text", ns: "svg", tag: "svg", scripting: true}
	good := &testCase{text: "<p>x", scripting: true}
	if s, p := parseCase(bad); !p {
		t.Fatalf("x/net/html no longer panics on the seed 33 case (got %q): drop this test's premise", s)
	}
	got := keepRandom([]*testCase{bad, good})
	if len(got) != 1 || got[0] != good {
		t.Fatalf("keepRandom kept %d cases, want only the one Go parses", len(got))
	}
}
