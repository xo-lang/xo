package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestXoTestSince (decision 0701): `xo test --since=BASE` runs the tests
// a change since the git revision BASE reaches: none for no change or a
// change to a file that is not a source, the test naming a changed
// function and, transitively, the test naming its caller, a new test, and
// every test for a source outside the module's tests' files (xo.mod).
func TestXoTestSince(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	dir := filepath.Join(tmp, "m")
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.com", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	lib := "fn double(x: Int) -> Int { x * 2 }\n\nfn quad(x: Int) -> Int { double(double(x)) }\n\nfn greet(s: Str) -> Str { \"hi ${s}\" }\n"
	tests := "test \"double\" { expect(double(2)) == 4 }\n\ntest \"quad\" { expect(quad(1)) == 4 }\n\ntest \"greet\" { expect(greet(\"a\")) == \"hi a\" }\n"
	write("xo.mod", "module example.com/since\n")
	write("lib.xo", lib)
	write("lib_test.xo", tests)
	write("README.md", "since\n")
	git("init", "-q")
	git("add", "-A")
	git("commit", "-qm", "base")
	xt := func(what, want string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, []string{"test", "--backend=" + backends[0], "--since=HEAD", dir}, &out, &errb)
		got := out.String() + errb.String()
		if code != 0 || !strings.Contains(got, want) {
			t.Errorf("%s: want exit 0 and %q, exit %d:\n%s", what, want, code, got)
		}
	}
	xt("no change", "ok: 0 passed")
	write("README.md", "since, edited\n")
	xt("a change to a file that is not a source", "ok: 0 passed")
	write("lib.xo", strings.Replace(lib, "x * 2", "x + x", 1))
	xt("double changed: double and its caller quad", "ok: 2 passed")
	// --json: the selection's line, each test with why
	{
		var out, errb bytes.Buffer
		run(t, []string{"test", "--backend=" + backends[0], "--since=HEAD", "--json", dir}, &out, &errb)
		got := out.String()
		if !strings.Contains(got, `{"name":"double","why":"names double, changed"}`) || !strings.Contains(got, `{"name":"quad","why":"names quad, which names double"}`) {
			t.Errorf("--since --json: want double and quad with why, got:\n%s%s", got, errb.String())
		}
	}
	// a bare --since: the merge base with origin/main, else main (the
	// commit here), so the same two tests
	xtArgs := func(what, want string, args ...string) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, append([]string{"test", "--backend=" + backends[0]}, append(args, dir)...), &out, &errb)
		if got := out.String() + errb.String(); code != 0 || !strings.Contains(got, want) {
			t.Errorf("%s: want exit 0 and %q, exit %d:\n%s", what, want, code, got)
		}
	}
	xtArgs("bare --since", "ok: 2 passed", "--since")
	write("lib.xo", lib)
	write("lib_test.xo", tests+"\ntest \"new\" { expect(1) == 1 }\n")
	xt("a new test", "ok: 1 passed")
	write("lib_test.xo", tests)
	write("xo.mod", "module example.com/since\n\n")
	xt("xo.mod changed: every test", "ok: 3 passed")
	var out, errb bytes.Buffer
	if code := run(t, []string{"test", "--since=no-such-rev", dir}, &out, &errb); code != 2 || !strings.Contains(errb.String(), "no such commit") {
		t.Errorf("--since=no-such-rev: want exit 2 and no such commit, exit %d:\n%s%s", code, out.String(), errb.String())
	}
}

// TestXoTestStdModuleResultCache (decision 0701, 0439 stage 2): an
// unchanged `xo test --std-module` run of a module whose tests use only
// the fakes replays, an edit to the module's source runs it again, and a
// module under std/native/llrt (system calls) always runs.
func TestXoTestStdModuleResultCache(t *testing.T) {
	if testing.Short() {
		t.Skip("builds test binaries")
	}
	backends := nativeBackends(t)
	tmp := t.TempDir()
	setenv(t, "XO_BUILD_DIR", filepath.Join(tmp, "build"))
	setenv(t, "XO_BIN_CACHE", filepath.Join(tmp, "bin"))
	setenv(t, "XO_BACKEND_CACHE_DIR", filepath.Join(tmp, "backend"))
	std := filepath.Join(tmp, "std")
	if err := os.CopyFS(std, os.DirFS(xoStd())); err != nil {
		t.Fatal(err)
	}
	setenv(t, "XO_STD", std)
	xt := func(what, module string, cached bool) {
		t.Helper()
		var out, errb bytes.Buffer
		code := run(t, []string{"test", "--backend=" + backends[0], "--std-module=" + module}, &out, &errb)
		got := out.String()
		if code != 0 || !strings.Contains(got, "ok: ") || strings.Contains(got, "(cached)") != cached {
			t.Errorf("%s: want exit 0, cached %v; exit %d:\n%s%s", what, cached, code, got, errb.String())
		}
	}
	xt("base64: first run", "base64", false)
	xt("base64: unchanged", "base64", true)
	src := filepath.Join(std, "base64.xo")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, append(b, []byte("\n// edited\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	xt("base64: source edited", "base64", false)
	// --since with --std-module takes no directory (the child is
	// `xo test --std-module=M` with the selection)
	var out, errb bytes.Buffer
	if run(t, []string{"test", "--backend=" + backends[0], "--std-module=base64", "--since=HEAD"}, &out, &errb); strings.Contains(errb.String(), "no directories") {
		t.Errorf("--std-module=base64 --since=HEAD: a usage error:\n%s%s", out.String(), errb.String())
	}
	xt("native/llrt/scan: first run", "native/llrt/scan", false)
	xt("native/llrt/scan: unchanged, still run", "native/llrt/scan", false)
}
