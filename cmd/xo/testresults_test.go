package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestXoTestResultCache (decision 0439): a passed run of an unchanged
// native test binary is replayed with " (cached)", and not with
// --count=1, XO_TEST_CACHE=off, another seed, a failing test, or --real
// (a --real test whose input file changed sees the change).
func TestXoTestResultCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	write := func(dir, name, text string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	xt := func(args ...string) (int, string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, append([]string{"test", "--backend=" + backends[0]}, args...), &out, &errb)
		return code, out.String() + errb.String()
	}
	pass := filepath.Join(tmp, "pass")
	write(pass, "xo.mod", "module example.com/pass\n")
	write(pass, "main.xo", "fn main(os: Os) {}\n\ntest \"one\" {\n  expect(1 + 1) == 2\n}\n\ntest \"fs\" {\n  os.fs.write(\"a\", b\"x\").or_fault(\"w\")\n  expect(os.fs.read(\"a\").or_fault(\"r\")) == b\"x\"\n}\n")
	steps := []struct {
		what   string
		env    string
		args   []string
		cached bool
	}{
		{"first run", "", nil, false},
		{"unchanged", "", nil, true},
		{"--count=1", "", []string{"--count=1"}, false},
		{"another seed", "", []string{"--seed=7"}, false},
		{"that seed again", "", []string{"--seed=7"}, true},
		// last: setenv's value stays in the test's environment
		{"XO_TEST_CACHE=off", "XO_TEST_CACHE=off", nil, false},
	}
	for _, s := range steps {
		if s.env != "" {
			k, v, _ := strings.Cut(s.env, "=")
			setenv(t, k, v)
		}
		code, out := xt(append(s.args, pass)...)
		if code != 0 || !strings.Contains(out, "ok: 2 passed, 0 failed, 0 skipped") {
			t.Fatalf("%s: exit %d\n%s", s.what, code, out)
		}
		if got := strings.Contains(out, "(cached)"); got != s.cached {
			t.Fatalf("%s: cached %v, want %v\n%s", s.what, got, s.cached, out)
		}
	}
	setenv(t, "XO_TEST_CACHE", "")
	// a failing run is never stored
	fail := filepath.Join(tmp, "fail")
	write(fail, "xo.mod", "module example.com/fail\n")
	write(fail, "main.xo", "fn main(os: Os) {}\n\ntest \"bad\" {\n  expect(1) == 2\n}\n")
	for i := 0; i < 2; i++ {
		if code, out := xt(fail); code != 1 || strings.Contains(out, "(cached)") {
			t.Fatalf("failing run %d: exit %d\n%s", i, code, out)
		}
	}
	// --real is never stored: a real file the test reads changes between
	// two runs of the same binary, and the second run sees it
	real := filepath.Join(tmp, "real")
	write(real, "xo.mod", "module example.com/real\n")
	write(real, "data.txt", "one\n")
	write(real, "main.xo", "fn main(os: Os) {}\n\ntest \"reads\" uses fs.read {\n  expect(os.fs.read_text(\"data.txt\")?) == \"one\\n\"\n}\n")
	chdir(t, real)
	if code, out := xt("--real", "."); code != 0 || strings.Contains(out, "(cached)") {
		t.Fatalf("--real first run: exit %d\n%s", code, out)
	}
	write(real, "data.txt", "two\n")
	if code, out := xt("--real", "."); code != 1 || strings.Contains(out, "(cached)") {
		t.Fatalf("--real after the input changed: want the test run and failing, exit %d\n%s", code, out)
	}
}

// TestXoTestResultCacheEdits (decision 0439): after an edit to anything
// the tests see, the rerun runs them, never a replay of the earlier
// result: the test body, a function the test calls, an embedded file,
// and a std function (std built from a copy). On linux/arm64 an edit
// replayed "ok" while the result key left the sources out (2026-10-30).
func TestXoTestResultCacheEdits(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	setenv(t, "XO_TEST_CACHE", "")
	write := func(dir, name, text string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// xt runs the package's tests and checks the exit code and whether
	// the result was replayed (cached < 0: either)
	xt := func(what, dir string, code, cached int) {
		t.Helper()
		var out, errb bytes.Buffer
		got := run(t, []string{"test", "--backend=" + backends[0], dir}, &out, &errb)
		text := out.String() + errb.String()
		if got != code {
			t.Fatalf("%s: exit %d, want %d\n%s", what, got, code, text)
		}
		if c := strings.Contains(text, "(cached)"); cached >= 0 && c != (cached == 1) {
			t.Fatalf("%s: cached %v, want %v\n%s", what, c, cached == 1, text)
		}
	}
	pkg := filepath.Join(tmp, "pkg")
	write(pkg, "xo.mod", "module example.com/pkg\n")
	write(pkg, "data.txt", "one\n")
	src := func(body, add string) string {
		return "const DATA: Str = embed(\"data.txt\")\n\nfn add(a: Int, b: Int) -> Int { " + add + " }\n\nfn main(os: Os) {}\n\ntest \"add\" {\n  " + body + "\n}\n\ntest \"data\" {\n  expect(DATA) == \"one\\n\"\n}\n"
	}
	write(pkg, "main.xo", src("expect(add(1, 1)) == 2", "a + b"))
	xt("first run", pkg, 0, 0)
	xt("unchanged", pkg, 0, 1)
	write(pkg, "main.xo", src("expect(add(2, 2)) == 5", "a + b"))
	xt("test body edited to fail", pkg, 1, 0)
	write(pkg, "main.xo", src("expect(add(2, 2)) == 4", "a + b"))
	xt("test body edited to pass", pkg, 0, 0)
	xt("that body unchanged", pkg, 0, 1)
	write(pkg, "main.xo", src("expect(add(2, 2)) == 4", "a + b + 1"))
	xt("called function edited", pkg, 1, 0)
	// the same inputs as a passed run: a replay is right, a run is too
	write(pkg, "main.xo", src("expect(add(2, 2)) == 4", "a + b"))
	xt("called function restored", pkg, 0, -1)
	xt("restored, unchanged", pkg, 0, 1)
	write(pkg, "data.txt", "two\n")
	xt("embedded file edited", pkg, 1, 0)
	write(pkg, "data.txt", "one\n")
	xt("embedded file restored", pkg, 0, -1)

	// a std function the test calls, edited in a copy of std
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	std := filepath.Join(tmp, "std")
	if err := os.CopyFS(std, os.DirFS(filepath.Join(root, "std"))); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_STD", std)
	uses := filepath.Join(tmp, "uses")
	write(uses, "xo.mod", "module example.com/uses\n")
	// Go's fnv.New32 of "a"
	write(uses, "main.xo", "use std/fnv\n\nfn main(os: Os) {}\n\ntest \"fnv\" {\n  expect(fnv.sum32(b\"a\")) == 84696446\n}\n")
	xt("std: first run", uses, 0, 0)
	xt("std: unchanged", uses, 0, 1)
	f := filepath.Join(std, "fnv.xo")
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	const from = "fn sum32(data: Bytes) -> Int { update32(OFFSET32, data) }"
	if !bytes.Contains(b, []byte(from)) {
		t.Fatalf("%s has no %q", f, from)
	}
	if err := os.WriteFile(f, bytes.Replace(b, []byte(from), []byte("fn sum32(data: Bytes) -> Int { update32(OFFSET32, data) + 1 }"), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	xt("std function edited", uses, 1, 0)
}
