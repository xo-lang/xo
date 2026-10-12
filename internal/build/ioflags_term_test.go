package build

import (
	"slices"
	"testing"
)

// TestIOFlagsTerminal: a native program that asks whether a stream is a
// terminal or for its width, without reading standard input, compiles
// xo_stdin.c (XO_STDIN), which defines xo_isatty and xo_term_width; it
// failed to link before (found by `xo audit` in Xo, decision 0178
// stage 4).
func TestIOFlagsTerminal(t *testing.T) {
	for _, code := range []string{"bl _xo_isatty", "call i64 @xo_term_width(", "bl _xo_tw_set_tty"} {
		if !slices.Contains(IOFlags([]byte(code)), "-DXO_STDIN") {
			t.Errorf("IOFlags(%q) lacks -DXO_STDIN", code)
		}
	}
}
