package rt

import "testing"

// TestLinkEscapes pins which symbolic link targets a sub capability
// refuses (std.md 3.1, decision 0163).
func TestLinkEscapes(t *testing.T) {
	cases := []struct {
		link, target string
		escapes      bool
	}{
		{"a", "b", false},
		{"a", "../b", true},
		{"d/a", "../b", false},
		{"d/a", "../../b", true},
		{"d/e/a", "../x/../b", false},
		{"a", "/etc/passwd", true},
		{"a", "C:/x", true},
		{"a", "./b/../c", false},
		{"a", "b/../../c", true},
	}
	for _, c := range cases {
		if got := linkEscapes(c.link, c.target); got != c.escapes {
			t.Errorf("linkEscapes(%q, %q) = %v, want %v", c.link, c.target, got, c.escapes)
		}
	}
}
