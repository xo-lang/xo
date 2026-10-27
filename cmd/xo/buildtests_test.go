package main

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestBuildSkipsTestFiles checks decision 0348: `xo build` and `xo run`
// read no _test.xo file, so a type error there does not stop them, a
// helper declared only in a test file does not exist for them (XO0601),
// and a module only a test file uses is never fetched; `xo check` and
// `xo test` still read the test files and report their errors.
func TestBuildSkipsTestFiles(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	main := "fn main(os: Os) {\n  os.stdio.println(\"hello\")\n}\n"
	dir := writeFiles(t, map[string]string{
		"broken/xo.mod":        "module broken\n",
		"broken/main.xo":       main,
		"broken/main_test.xo":  "test \"broken\" {\n  let n: Int = \"text\"\n  expect(n) == 1\n}\n",
		"helper/xo.mod":        "module helper\n",
		"helper/main.xo":       "fn main(os: Os) {\n  os.stdio.println(shout(\"hi\"))\n}\n",
		"helper/main_test.xo":  "fn shout(s: Str) -> Str { s.to_upper() }\n\ntest \"shout\" {\n  expect(shout(\"a\")) == \"A\"\n}\n",
		"useonly/xo.mod":       "module useonly\n",
		"useonly/main.xo":      main,
		"useonly/main_test.xo": "use example.invalid/nowhere\n\ntest \"t\" {\n  expect(nowhere.f()) == 1\n}\n",
	})
	setenv(t, "XO_OFFLINE", "1")
	xo := func(args ...string) (int, string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, args, &out, &errb)
		return code, out.String() + errb.String()
	}
	for _, m := range []string{"broken", "useonly"} {
		d := filepath.Join(dir, m)
		if code, out := xo("build", "-o", filepath.Join(t.TempDir(), m), d); code != 0 {
			t.Errorf("xo build %s: exit %d\n%s", m, code, out)
		}
		if code, out := xo("run", d); code != 0 || out != "hello\n" {
			t.Errorf("xo run %s: exit %d\n%s", m, code, out)
		}
		if code, out := xo("check", d); code != 1 {
			t.Errorf("xo check %s: exit %d, want 1 (the test file's error)\n%s", m, code, out)
		}
		if code, out := xo("test", d); code == 0 {
			t.Errorf("xo test %s: exit %d, want a failure\n%s", m, code, out)
		}
	}
	if code, out := xo("check", filepath.Join(dir, "broken")); !strings.Contains(out, "main_test.xo:2:") {
		t.Errorf("xo check broken: exit %d, no error in main_test.xo\n%s", code, out)
	}
	h := filepath.Join(dir, "helper")
	if code, out := xo("build", "-o", filepath.Join(t.TempDir(), "helper"), h); code != 1 || !strings.Contains(out, "XO0601") || !strings.Contains(out, "shout") {
		t.Errorf("xo build helper: exit %d, want XO0601 for shout\n%s", code, out)
	}
	if code, out := xo("check", h); code != 0 {
		t.Errorf("xo check helper: exit %d\n%s", code, out)
	}
	if code, out := xo("test", h); code != 0 || !strings.Contains(out, "1 passed") {
		t.Errorf("xo test helper: exit %d\n%s", code, out)
	}
}

// TestBuildInlineTestBlocks: `xo build` and `xo run` skip inline test
// blocks (decision 0389, as 0348 skips _test.xo files): a block with a
// type error builds and runs, and `xo check` and `xo test` still fail on
// it; a valid block passes `xo test`; and a build of a file with a test
// block parses no more files than the same file without it (no
// std/testing, whose imports bring http and sql).
func TestBuildInlineTestBlocks(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	main := "fn main(os: Os) {\n  os.stdio.println(\"hello\")\n}\n"
	dir := writeFiles(t, map[string]string{
		"plain/xo.mod":  "module plain\n",
		"plain/main.xo": main,
		"good/xo.mod":   "module good\n",
		"good/main.xo":  main + "\ntest \"inline\" {\n  let c = os.clock\n  expect(c.sleeps().len()) == 0\n}\n",
		"bad/xo.mod":    "module bad\n",
		"bad/main.xo":   main + "\ntest \"inline\" {\n  let n: Int = \"text\"\n  expect(n) == 1\n}\n",
	})
	xo := func(args ...string) (int, string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, args, &out, &errb)
		return code, out.String() + errb.String()
	}
	parsed := regexp.MustCompile(`cache parse \d+/(\d+)`)
	files := func(d string) string {
		t.Helper()
		code, out := xo("build", "--phase-times", "-o", filepath.Join(t.TempDir(), "p"), filepath.Join(dir, d))
		if code != 0 {
			t.Fatalf("xo build %s: exit %d\n%s", d, code, out)
		}
		m := parsed.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("xo build %s: no cache parse line\n%s", d, out)
		}
		return m[1]
	}
	if p, g := files("plain"), files("good"); p != g {
		t.Errorf("a build of a file with a test block parsed %s files, without it %s: want the same (no std/testing)", g, p)
	}
	g := filepath.Join(dir, "good")
	if code, out := xo("test", g); code != 0 || !strings.Contains(out, "1 passed") {
		t.Errorf("xo test good: exit %d\n%s", code, out)
	}
	b := filepath.Join(dir, "bad")
	if code, out := xo("build", "-o", filepath.Join(t.TempDir(), "bad"), b); code != 0 {
		t.Errorf("xo build bad: exit %d, want 0 (a build does not check test blocks)\n%s", code, out)
	}
	if code, out := xo("run", b); code != 0 || out != "hello\n" {
		t.Errorf("xo run bad: exit %d, want 0 and hello\n%s", code, out)
	}
	for _, cmd := range []string{"check", "test"} {
		if code, out := xo(cmd, b); code == 0 || !strings.Contains(out, "main.xo:6:") {
			t.Errorf("xo %s bad: exit %d, want the test block's error at main.xo:6\n%s", cmd, code, out)
		}
	}
}
