package xotool

import (
	"strings"
	"testing"
)

// TestFirstErrors: the pre-build check's error report keeps the error
// lines, each one's note, and xo check's summary line.
func TestFirstErrors(t *testing.T) {
	out := "compiler/xo/a.xo:3:5: error XO0601: `x` not found\n  note: did you mean `y`\ncompiler/xo/b.xo:1:1: warning XO0305: hides\n1 module(s), 1 error(s), 1 warning(s)\n"
	got := firstErrors(out, 20)
	want := "compiler/xo/a.xo:3:5: error XO0601: `x` not found\n  note: did you mean `y`\n1 module(s), 1 error(s), 1 warning(s)"
	if got != want {
		t.Errorf("firstErrors =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(firstErrors(out, 0), "XO0601") {
		t.Errorf("firstErrors with n=0 kept an error")
	}
}
