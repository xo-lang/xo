package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestOptionTryInTests checks decision 0347 on testdata/testopt/tests:
// `x?` on an Option directly in a test body (nested blocks, `if`,
// `match`, a property test, mixed with a Result's `?`) fails the test
// with `error: missing value: EXPR` on the Go backend and every native
// one, a property test's missing value is shrunk like any failure, and a
// fn literal in a test keeps its own rules (it becomes Option returning).
func TestOptionTryInTests(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	dir := filepath.Join("testdata", "testopt", "tests")
	wants := []string{
		"--- FAIL: none fails (main.xo:15:1, seed 1)\n    error: missing value: find(5)\n",
		"--- FAIL: nested (main.xo:20:1, seed 1)\n    error: missing value: m.get(\"zz\")\n",
		"--- FAIL: mixed (main.xo:34:1, seed 1)\n    error: missing value: find(b + 1)\n",
		"--- FAIL: prop (main.xo:41:1, seed 1)\n    property failed on case 1; shrunk in ",
		"    n = 3\n    error: missing value: find(n)\n",
		"FAIL: 2 passed, 4 failed, 0 skipped\n",
	}
	check := func(name string, code int, out string) {
		t.Helper()
		if code != 1 {
			t.Errorf("%s: exit %d, want 1\n%s", name, code, out)
		}
		for _, w := range wants {
			if !strings.Contains(out, w) {
				t.Errorf("%s: no %q in\n%s", name, w, out)
			}
		}
		for _, bad := range []string{"some passes", "closure keeps its rules"} {
			if strings.Contains(out, "FAIL: "+bad) {
				t.Errorf("%s: %q failed\n%s", name, bad, out)
			}
		}
	}
	code, out := xoTest(t, "--backend=go", dir)
	check("go", code, out)
	for _, be := range nativeBackends(t) {
		code, out := xoTest(t, "--backend="+be, dir)
		check(be, code, out)
	}
}

// TestOptionTryHelpers checks that decision 0347 is only for test
// bodies: in testdata/testopt/helper a `_test.xo` helper's `opt?` is
// still XO0204 (returns a Result) or XO0205 (cannot fail), each with a
// note that names test blocks, and production code's XO0204 has no such
// note.
func TestOptionTryHelpers(t *testing.T) {
	t.Parallel()
	var out, errb bytes.Buffer
	run(t, []string{"check", "--json", filepath.Join("testdata", "testopt", "helper")}, &out, &errb)
	var res struct {
		Diagnostics []struct {
			Code string
			Span struct {
				File string
				Line int
			}
			Notes []struct{ Message string }
		}
	}
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("%v\n%s%s", err, out.String(), errb.String())
	}
	got := map[string]bool{}
	for _, d := range res.Diagnostics {
		note := false
		for _, n := range d.Notes {
			if strings.Contains(n.Message, "allowed directly in test blocks") {
				note = true
			}
		}
		got[d.Code+" "+filepath.Base(d.Span.File)+" "+map[bool]string{true: "note", false: "plain"}[note]] = true
	}
	for _, w := range []string{"XO0204 lib_test.xo note", "XO0205 lib_test.xo note", "XO0204 lib.xo plain"} {
		if !got[w] {
			t.Errorf("no %s in %v\n%s", w, got, out.String())
		}
	}
	if len(res.Diagnostics) != 3 {
		t.Errorf("%d diagnostics, want 3\n%s", len(res.Diagnostics), out.String())
	}
}
