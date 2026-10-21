package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dumpFnProg = `fn sq(x: Int) -> Int { x * x + 1 }

fn main(os: Os) {
  let n = (os.env.get("N") ?? "3").len()
  os.stdio.println("${sq(n)}")
}
`

// TestDumpFn covers `xo dev dump ssa --fn=NAME --stage=S` and `xo dev dump
// asm --ssa=rc|inline --fn=NAME`: only the named functions, a std function
// main does not reach (declared as a root), the module after the passes
// (rc shows the borrow inference), and after the inliner (the call of sq is
// replaced by its body).
func TestDumpFn(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module example.com/df\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(dumpFnProg), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	dump := func(args ...string) string {
		t.Helper()
		var out, errb bytes.Buffer
		if code := runXo(t, append([]string{"dev", "dump"}, args...), &out, &errb); code != 0 {
			t.Fatalf("xo dev dump %v: exit %d\n%s", args, code, errb.String())
		}
		return out.String()
	}
	// a std function the program never calls, after rc: only it, with a
	// borrowed parameter
	s := dump("ssa", "--fn=flate.InStream.push", "--stage=rc", ".")
	if !strings.HasPrefix(s, "== stage rc\nfunc flate.InStream.push(") || strings.Count(s, "\nfunc ") != 1 || !strings.Contains(s, "borrow v1") {
		t.Errorf("std function after rc:\n%.600s", s)
	}
	// the build stage keeps the module's functions out of the answer too
	if s := dump("ssa", "--fn=sq", "."); !strings.Contains(s, "func sq(") || strings.Contains(s, "func main(") {
		t.Errorf("--fn=sq:\n%.400s", s)
	}
	if s := dump("ssa", "--fn=nosuchfunction", "."); !strings.Contains(s, "== no function matches nosuchfunction") {
		t.Errorf("no match:\n%.400s", s)
	}
	// after rc main calls sq; after the inliner the body is in main
	rc := dump("asm", "--ssa=rc", "--fn=main", ".")
	if !strings.Contains(rc, "== ssa after rc\n") || !strings.Contains(rc, "call sq") {
		t.Errorf("main after rc lacks the call of sq:\n%.800s", rc)
	}
	in := dump("asm", "--ssa=inline", "--fn=main", ".")
	if !strings.Contains(in, "== ssa after inline\n") || strings.Contains(in, "call sq") || !strings.Contains(in, " mul ") {
		t.Errorf("main after inline still calls sq:\n%.800s", in)
	}
}
