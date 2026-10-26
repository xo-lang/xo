package runfilter

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"TestA", []string{"TestA"}},
		{"TestA/x$", []string{"TestA", "x$"}},
		{"TestA/(a/b)", []string{"TestA", "(a/b)"}},
		{"TestA/[/]x", []string{"TestA", "[/]x"}},
		{`TestA/a\/b/c`, []string{"TestA", `a\/b`, "c"}},
	} {
		if got := split(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("split(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSub runs under go test's own -run, so it checks the whole-set
// case it always sees, and the matching on patterns set by hand.
func TestSub(t *testing.T) {
	keep, filtered := Sub(t)
	if t.Name() == "TestSub" && !filtered && !keep("anything") {
		t.Errorf("no subtest pattern: keep(anything) is false")
	}
	run := split("TestSelfhostWasm/var_patterns$")
	re := compile(t, run[1])
	if !re.MatchString("var_patterns") || re.MatchString("var_patterns_more") {
		t.Errorf("var_patterns$ does not select var_patterns alone")
	}
}
