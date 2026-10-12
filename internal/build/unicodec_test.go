package build

import (
	"slices"
	"testing"
)

// TestIOFlagsUnicodeC checks that code calling the C path of the Unicode
// operations compiles the runtime with -DXO_UNICODE_C (decision 0180),
// and that the default (Xo) path, whose code calls xo_rune_is_space and
// the rs_* primitives, does not.
func TestIOFlagsUnicodeC(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_rune_to_case\n", true},
		{"call ptr @xo_str_lower(ptr %1, i64 %2, ptr %3)\n", true},
		{"bl _xo_unicode_ranges\n", true},
		{"bl _xo_rune_is_space\nbl _xo_rs_append_bytes\nbl _xo_bytes_span\n", false},
		// the names as the program's string data (xo1's own tables)
		{"\t.ascii \"xo_str_lower\"\n\t.ascii \"xo_rune_quote\"\n", false},
		{"@.s1 = private constant [12 x i8] c\"xo_str_lower\"\n", false},
		{"\t.ascii \"xo_str_lower\"\nbl xo_str_lower\n", true},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_UNICODE_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_UNICODE_C: %v, want %v", c.code, got, c.want)
		}
	}
}
