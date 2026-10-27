package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestInlineTestBlockTestingTypes: a test block in a program file (not a
// _test.xo file) names std/testing's types as `testing.X` in a type
// position, as a _test.xo file can; it was XO0601 (an unresolved module).
func TestInlineTestBlockTestingTypes(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	dir := writeFiles(t, map[string]string{
		"xo.mod":  "module tt\n",
		"main.xo": "fn main(os: Os) {\n  os.stdio.println(\"hello\")\n}\n\ntest \"typed\" {\n  let c: testing.FakeClock = os.clock\n  expect(c.sleeps().len()) == 0\n}\n",
	})
	for _, cmd := range []string{"check", "test"} {
		var out, errb bytes.Buffer
		code := run(t, []string{cmd, dir}, &out, &errb)
		if code != 0 {
			t.Errorf("xo %s: exit %d\n%s%s", cmd, code, out.String(), errb.String())
		}
		if cmd == "test" && !strings.Contains(out.String()+errb.String(), "1 passed") {
			t.Errorf("xo test: want 1 passed\n%s%s", out.String(), errb.String())
		}
	}
}
