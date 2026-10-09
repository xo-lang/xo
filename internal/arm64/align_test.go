package arm64

import (
	"strings"
	"testing"
)

func TestParseLayout(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Layout
		bad  bool
	}{
		{in: "", want: Layout{}},
		{in: "func=6,loop=5,fall=1,off=3,outer=1,pad=7", want: Layout{Func: 6, Loop: 5, LoopFall: true, LoopOff: 3, Outer: true, Pad: 7}},
		{in: " loop=4 , ", want: Layout{Loop: 4}},
		{in: "loop", bad: true},
		{in: "loop=x", bad: true},
		{in: "size=3", bad: true},
		{in: "func=13", bad: true},
	} {
		got, err := ParseLayout(tc.in)
		if (err != nil) != tc.bad || (!tc.bad && got != tc.want) {
			t.Errorf("ParseLayout(%q) = %+v, %v", tc.in, got, err)
		}
	}
}

// TestAlignLoops: the top of an innermost loop is the earliest label a
// later branch of the loop goes back to (L_c past a rotated check); a
// top fallen into gets a jump over its padding unless fall is set; a
// branch from a block outside the loop (a cold block laid out last) does
// not make a top.
func TestAlignLoops(t *testing.T) {
	body := strings.Join([]string{
		"_f:",
		"\tmov x0, #0",
		"Lb1:",
		"\tcmp x0, x1",
		"\tb.hs Lfault",
		"Lb1_c:",
		"\tadd x0, x0, #1",
		"\tcbz x2, Lb2",
		"Lb3:",
		"\tcmp x0, x1",
		"\tb.lo Lb1_c",
		"\tb Lb1",
		"\tret",
		"Lb2:",
		"\tb Lb1",
	}, "\n")
	loop := map[string]int{"Lb1": 0, "Lb3": 0}
	got := alignLoops(body, 6, false, 0, loop)
	want := strings.Replace(body, "\tb.hs Lfault\nLb1_c:", "\tb.hs Lfault\n\tb Lb1_c\n\t.p2align 6\nLb1_c:", 1)
	if got != want {
		t.Errorf("jump over:\n%s\nwant:\n%s", got, want)
	}
	got = alignLoops(body, 5, true, 2, loop)
	want = strings.Replace(body, "\tb.hs Lfault\nLb1_c:", "\tb.hs Lfault\n\t.p2align 5\n\tnop\n\tnop\nLb1_c:", 1)
	if got != want {
		t.Errorf("fall through:\n%s\nwant:\n%s", got, want)
	}
	if got := alignLoops(body, 2, false, 0, loop); got != body {
		t.Errorf("p 2 changed the body")
	}
	if got := alignLoops(body, 6, false, 0, map[string]int{"Lb1": 0}); got != body {
		t.Errorf("a branch from outside the loop made a top:\n%s", got)
	}
}
