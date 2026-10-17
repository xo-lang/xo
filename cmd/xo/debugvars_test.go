package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// debugVarsProg has parameters, a `var` carried around a loop, a `for`
// variable, and a `let` in the loop body.
const debugVarsProg = `struct Point { x: Int, y: Int }

fn sum(ps: List[Point], k: Int) -> Int {
  var total = 0
  for p in ps {
    let d = p.x * k + p.y
    total += d
  }
  total
}

fn main(os: Os) {
  let ps = [Point{x: 1, y: 2}, Point{x: 3, y: 4}]
  os.stdio.println("${sum(ps, 10)}")
}
`

// TestDebugVarsDump checks the SSA's source variables and binding
// records (decision 0252, stage 1) through `xo dev dump ssa
// --debug-vars`: every local of sum with its type, declaration line, and
// whether it is a parameter, and a binding record for each assignment
// and for the loop's join parameters.
func TestDebugVarsDump(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/dv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(debugVarsProg), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	var out, errb bytes.Buffer
	if code := runXo(t, []string{"dev", "dump", "ssa", "--debug-vars", "."}, &out, &errb); code != 0 {
		t.Fatalf("xo dev dump ssa: exit %d\n%s", code, errb.String())
	}
	i := strings.Index(out.String(), "func sum(")
	if i < 0 {
		t.Fatalf("no func sum in the dump:\n%s", out.String())
	}
	body := out.String()[i:]
	body = body[:strings.Index(body, "\n}\n")]
	for _, want := range []string{
		"; var 0 ps: List[Point] line 3:8 param",
		"; var 1 k: Int line 3:25 param",
		"; var 2 total: Int line 4:7",
		"; var 3 p: Point line 5:7",
		"; var 4 d: Int line 6:9",
		"; bind 2 = v", // total = 0
		" from 4:3",    // at its declaration
		" from 7:5",    // total += d
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dump of sum lacks %q:\n%s", want, body)
		}
	}
	// the loop carries total and k in join parameters: bound from the
	// start of the loop header (0:0)
	if n := strings.Count(body, "from 0:0"); n < 4 {
		t.Errorf("want at least 4 records from a block's start (ps, k, and the loop's k and total), got %d:\n%s", n, body)
	}
	// without the flag the dump has no records (the recorded dumps stay
	// as they were, decision 0217)
	out.Reset()
	if code := runXo(t, []string{"dev", "dump", "ssa", "."}, &out, &errb); code != 0 {
		t.Fatalf("xo dev dump ssa: exit %d\n%s", code, errb.String())
	}
	if strings.Contains(out.String(), "; var ") || strings.Contains(out.String(), "; bind ") {
		t.Errorf("the dump without --debug-vars shows variables")
	}
}
