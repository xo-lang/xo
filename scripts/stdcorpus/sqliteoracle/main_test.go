package main

import "testing"

// TestQuoteEscapesDollar: a text holding "${" is spelled so Xo reads it as
// text, not an interpolation (TestSqliteFuzz seed 148, 2026-11-18).
func TestQuoteEscapesDollar(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a${b}", `"a\${b}"`},
		{`x\$`, `"x\\\$"`},
		{"plain", `"plain"`},
	} {
		if got := q(c.in); got != c.want {
			t.Errorf("q(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
