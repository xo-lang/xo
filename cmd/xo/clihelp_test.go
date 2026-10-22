package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateHelp = flag.Bool("update-help", false, "rewrite testdata/clihelp from the shipped xo's help")

// clihelpDir is testdata/clihelp, absolute: the tests change directory.
var clihelpDir, _ = filepath.Abs(filepath.Join("testdata", "clihelp"))

// helpCommands are the user commands of spec/core.md 10.1, as
// compiler/cli/cmdhelp lists them.
var helpCommands = []string{"build", "run", "test", "check", "parse", "fmt", "fix", "vet", "query", "edit", "doc", "def", "refs", "type", "symbols", "explain", "init", "get", "mod", "vendor", "publish", "audit", "bind", "dump", "replay", "lsp", "mcp", "serve", "version"}

// shippedXo is the shipped xo (xo1, decision 0178 switch point B) whose
// command line the help tests drive: the binary the other tests run
// (cli_test.go: $XO_CLI, else the checkout's bin/xo, which finds std at
// <bin>/../std as a release's finds lib/xo/std), with no $XO_STD, since
// commands that check (TestCLIPattern) run from a directory with no std/
// (gotcha shipped-xo-test-needs-release-layout).
func shippedXo(t *testing.T) []string {
	t.Helper()
	cli, err := xoCommand()
	if err != nil {
		t.Fatalf("no xo to run: %v", err)
	}
	setenv(t, "XO_STD", "")
	return cli
}

// helpGolden is testdata/clihelp/NAME.txt.
func helpGolden(t *testing.T, name, got string) string {
	t.Helper()
	p := filepath.Join(clihelpDir, name+".txt")
	if *updateHelp {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return got
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run with -update-help)", err)
	}
	return string(b)
}

// dropUsage removes the usage a usage error of cmd ends with, so the Go
// and the Xo command lines compare on the error itself: Go's flag
// defaults ("Usage of NAME:" to the end) or the shipped xo's usage of
// cmd (testdata/clihelp/CMD.txt, decision 0178: the help is xo1's).
func dropUsage(cmd, s string) string {
	if i := strings.Index(s, "Usage of "); i >= 0 {
		return s[:i]
	}
	if b, err := os.ReadFile(filepath.Join(clihelpDir, cmd+".txt")); err == nil && len(b) > 0 {
		return strings.TrimSuffix(s, string(b))
	}
	return s
}

// TestCLIHelp pins the shipped xo's help: `xo help` and `xo <command> -h`
// (and --help) print the usage on standard output and exit 0, with no
// internal flag or xo1 name in it; `xo` alone prints the command list on
// standard error and exits 2; an unknown flag exits 2 with the error and
// the command's usage on standard error.
func TestCLIHelp(t *testing.T) {
	t.Parallel()
	cli := shippedXo(t)
	chdir(t, t.TempDir())
	xo := func(args ...string) (string, string, int) {
		var out, errOut bytes.Buffer
		code := execCLI(t, cli, args, &out, &errOut)
		return out.String(), errOut.String(), code
	}
	top, stderr, code := xo("help")
	if code != 0 || stderr != "" {
		t.Errorf("xo help: exit %d, stderr %q", code, stderr)
	}
	if want := helpGolden(t, "help", top); top != want {
		t.Errorf("xo help:\n%s\nwant:\n%s", top, want)
	}
	for _, c := range helpCommands {
		if !strings.Contains(top, "\n  "+c+" ") {
			t.Errorf("xo help does not list %s", c)
		}
	}
	for _, internal := range []string{"xo1", "--std", "--wd", "--cc", "--go-tree", "--phase-times", "--asm", "self hosted"} {
		if strings.Contains(top, internal) {
			t.Errorf("xo help names %q", internal)
		}
	}
	if out, stderr, code := xo(); code != 2 || out != "" || stderr != top {
		t.Errorf("xo alone: exit %d, stdout %q, stderr %q; want 2 and the command list on stderr", code, out, stderr)
	}
	if out, _, code := xo("--help"); code != 0 || out != top {
		t.Errorf("xo --help: exit %d, %q", code, out)
	}
	if out, stderr, code := xo("frob"); code != 2 || out != "" || !strings.HasPrefix(stderr, "xo: unknown command \"frob\"\n") {
		t.Errorf("xo frob: exit %d, stdout %q, stderr %q", code, out, stderr)
	}
	for _, c := range helpCommands {
		t.Run(c, func(t *testing.T) {
			usage, stderr, code := xo(c, "-h")
			if code != 0 || stderr != "" {
				t.Errorf("xo %s -h: exit %d, stderr %q", c, code, stderr)
			}
			if want := helpGolden(t, c, usage); usage != want {
				t.Errorf("xo %s -h:\n%s\nwant:\n%s", c, usage, want)
			}
			if !strings.HasPrefix(usage, "usage: xo "+c) {
				t.Errorf("xo %s -h does not start with its usage line: %q", c, usage)
			}
			for _, internal := range []string{"xo1", "--std", "--wd", "--cc=", "--go-tree", "--phase-times", "--asm"} {
				if strings.Contains(usage, internal) {
					t.Errorf("xo %s -h names %q", c, internal)
				}
			}
			for _, args := range [][]string{{c, "--help"}, {"help", c}} {
				if out, _, code := xo(args...); code != 0 || out != usage {
					t.Errorf("xo %s: exit %d, %q", strings.Join(args, " "), code, out)
				}
			}
			out, stderr, code := xo(c, "--bogus")
			if code != 2 || out != "" || !strings.HasSuffix(stderr, usage) {
				t.Errorf("xo %s --bogus: exit %d, stdout %q, stderr %q; want 2 and the usage on stderr", c, code, out, stderr)
			}
		})
	}
	// help after a subcommand or an operation, and before `--`
	for _, args := range [][]string{{"edit", "rename", "--help"}, {"mod", "tidy", "-h"}, {"query", "def", "-h"}, {"serve", "status", "-help"}, {"test", "--run", "x", "-h"}, {"build", ".", "-h"}} {
		if out, _, code := xo(args...); code != 0 || !strings.HasPrefix(out, "usage: xo "+args[0]) {
			t.Errorf("xo %s: exit %d, %q", strings.Join(args, " "), code, out)
		}
	}
	// run's -h after the directory is the program's
	if out, _, code := xo("run", "nodir", "-h"); code == 0 || strings.HasPrefix(out, "usage: xo run") {
		t.Errorf("xo run nodir -h: exit %d, %q (want the build of nodir to fail)", code, out)
	}
	// the pattern DIR/... (decision 0210) is TestCLIPattern's; another
	// form is a usage error, and run takes no pattern
	for _, c := range []string{"check", "vet", "fmt", "fix", "doc", "parse", "build", "test", "audit"} {
		for _, p := range []string{"...", "a/.../b", "x..."} {
			out, stderr, code := xo(c, p)
			want := "xo " + c + ": " + p + ": only the pattern DIR/... is supported (for example xo " + c + " ./...), where DIR/... is every directory at or below DIR\n"
			if code != 2 || out != "" || stderr != want {
				t.Errorf("xo %s %s: exit %d, stdout %q, stderr %q", c, p, code, out, stderr)
			}
		}
	}
	if out, stderr, code := xo("run", "./..."); code != 2 || out != "" || stderr != "xo run: ./...: run takes one directory, not a package pattern\n" {
		t.Errorf("xo run ./...: exit %d, stdout %q, stderr %q", code, out, stderr)
	}
}

// TestCLINoStd checks that the shipped xo reports a std directory it
// cannot check against as a load error (exit 2) instead of faulting in the
// checker (exit 70, "prelude type missing: Int"): no prelude.xo there,
// or a prelude lacking a core type.
func TestCLINoStd(t *testing.T) {
	t.Parallel()
	cli := shippedXo(t)
	root := t.TempDir()
	for name, src := range map[string]string{
		"m/xo.mod":         "module example.com/m\nedition preview\n",
		"m/a.xo":           "pub fn f() -> Int {\n  1\n}\n",
		"empty/notes.txt":  "no prelude\n",
		"short/prelude.xo": "struct Str {}\nstruct Bool {}\n",
		"short/testing.xo": "",
	} {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, filepath.Join(root, "m"))
	for _, c := range []struct{ std, want string }{
		{"empty", "xo check: std library not found: no " + filepath.ToSlash(filepath.Join(root, "empty")) + "/prelude.xo"},
		{"short", "prelude.xo lacks Int, Never, Option, Result"},
	} {
		setenv(t, "XO_STD", filepath.ToSlash(filepath.Join(root, c.std)))
		var out, errOut bytes.Buffer
		code := execCLI(t, cli, []string{"check", "."}, &out, &errOut)
		if code != 2 || !strings.Contains(errOut.String(), c.want) || strings.Contains(errOut.String(), "fault") {
			t.Errorf("XO_STD=%s xo check .: exit %d, stdout %q, stderr %q; want exit 2 and %q", c.std, code, out.String(), errOut.String(), c.want)
		}
	}
}

// TestCLIPattern checks the pattern DIR/... of the shipped xo (decision
// 0210, spec/core.md 10.1): every command that takes directories reads
// DIR/... as DIR, which already covers the directories below it, so the
// two print the same; directories named testdata or starting with . or _
// are skipped; a DIR/... with no .xo file at or below DIR is an error
// (exit 1).
func TestCLIPattern(t *testing.T) {
	t.Parallel()
	cli := shippedXo(t)
	root := t.TempDir()
	bad := "pub fn f() -> Int {\n  \"x\"\n}\n"
	files := map[string]string{
		"m/xo.mod":           "module example.com/m\nedition preview\n",
		"m/main.xo":          "use example.com/m/sub\n\nfn main(os: Os) {\n  os.stdio.println(\"${sub.g()}\")\n}\n\ntest \"root\" {\n  expect(1) == 1\n}\n",
		"m/sub/a.xo":         "pub fn g() -> Int {\n  2\n}\n\ntest \"sub\" {\n  expect(g()) == 2\n}\n",
		"m/sub/deep/b.xo":    "pub fn h() -> Int {\n  3\n}\n",
		"m/_skip/a.xo":       bad,
		"m/.hidden/a.xo":     bad,
		"m/testdata/x/a.xo":  bad,
		"m/empty/notes.txt":  "no sources\n",
		"m/broken/deep/c.xo": bad,
	}
	for name, src := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, filepath.Join(root, "m"))
	setenv(t, "XO_BUILD_DIR", filepath.Join(root, "build"))
	xo := func(args ...string) string {
		var out, errOut bytes.Buffer
		code := execCLI(t, cli, args, &out, &errOut)
		return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, out.String(), errOut.String())
	}
	// a plain directory is that directory alone; DIR/... covers the
	// directories below it too (decision 0345)
	for _, c := range []struct {
		args       []string
		plain, pat string
	}{
		{[]string{"check", "sub"}, "1 module(s), 0 error(s)", "2 module(s), 0 error(s)"},
		{[]string{"vet", "sub"}, "1 module(s)", "2 module(s)"},
		{[]string{"parse", "sub"}, "1 file(s), 0 error(s)", "2 file(s), 0 error(s)"},
		{[]string{"check", "."}, "exit 0\n", "broken/deep/c.xo"},
	} {
		if got := xo(c.args...); !strings.Contains(got, c.plain) {
			t.Errorf("xo %s (that directory alone, want %q):\n%s", strings.Join(c.args, " "), c.plain, got)
		}
		pat := append(append([]string{}, c.args[:len(c.args)-1]...), c.args[len(c.args)-1]+"/...")
		if got := xo(pat...); !strings.Contains(got, c.pat) {
			t.Errorf("xo %s (want %q):\n%s", strings.Join(pat, " "), c.pat, got)
		}
	}
	// a plain directory with no .xo file names DIR/...
	for _, c := range []string{"check", "vet", "test", "fix"} {
		if got := xo(c, "broken"); !strings.Contains(got, "broken has no .xo files") || !strings.Contains(got, "broken/... covers the directories below it") {
			t.Errorf("xo %s broken:\n%s", c, got)
		}
	}
	if got := xo("check", "--json", "broken/..."); !strings.HasPrefix(got, "exit 1\n") || !strings.Contains(got, "broken/deep/c.xo") {
		t.Errorf("xo check --json broken/...:\n%s", got)
	}
	got := xo("check", "./...")
	if !strings.HasPrefix(got, "exit 1\n") || !strings.Contains(got, "broken/deep/c.xo") || strings.Contains(got, "_skip") || strings.Contains(got, ".hidden") || strings.Contains(got, "testdata") {
		t.Errorf("xo check ./... (want broken/deep, not _skip, .hidden, testdata):\n%s", got)
	}
	for _, c := range []string{"check", "test", "vet", "fmt", "doc"} {
		want := "exit 1\n--- stdout\n--- stderr\nxo " + c + ": empty/... matched no directory with .xo files\n"
		if got := xo(c, "empty/..."); got != want {
			t.Errorf("xo %s empty/...:\n%s\nwant:\n%s", c, got, want)
		}
	}
}

// TestCLIRunNoWalk covers decision 0345: xo run runs exactly the
// directory it is given and never a program found below it, and a DIR/...
// walk stops at its bound.
func TestCLIRunNoWalk(t *testing.T) {
	t.Parallel()
	cli := shippedXo(t)
	root := t.TempDir()
	files := map[string]string{
		"p/xo.mod":    "module example.com/p\nedition preview\n",
		"p/a/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"ran a\")\n}\n",
		"p/b/main.xo": "fn main(os: Os) {\n  os.stdio.println(\"ran b\")\n}\n",
		"p/lib/l.xo":  "pub fn f() -> Int {\n  1\n}\n",
	}
	for name, src := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chdir(t, filepath.Join(root, "p"))
	setenv(t, "XO_BUILD_DIR", filepath.Join(root, "build"))
	xo := func(args ...string) (int, string) {
		var out, errOut bytes.Buffer
		code := execCLI(t, cli, args, &out, &errOut)
		return code, out.String() + errOut.String()
	}
	// a parent of two programs: an error naming them, neither runs
	for _, args := range [][]string{{"run"}, {"run", "."}} {
		code, got := xo(args...)
		if code != 1 || strings.Contains(got, "ran ") || !strings.Contains(got, "has no .xo files") || !strings.Contains(got, "programs in the directories below: a, b") || !strings.Contains(got, "fix: xo run a") {
			t.Errorf("xo %s: exit %d:\n%s", strings.Join(args, " "), code, got)
		}
	}
	if code, got := xo("run", "lib"); code != 1 || !strings.Contains(got, "lib has no main function") {
		t.Errorf("xo run lib: exit %d:\n%s", code, got)
	}
	if code, got := xo("run", "a/main.xo"); code != 2 || !strings.Contains(got, "a/main.xo is a file, not a directory") || !strings.Contains(got, "fix: xo run a") {
		t.Errorf("xo run a/main.xo: exit %d:\n%s", code, got)
	}
	if code, got := xo("run", "./..."); code != 2 || strings.Contains(got, "ran ") {
		t.Errorf("xo run ./...: exit %d:\n%s", code, got)
	}
	if code, got := xo("run", "b"); code != 0 || got != "ran b\n" {
		t.Errorf("xo run b: exit %d:\n%s", code, got)
	}
	// the bound: more than 60,000 entries below DIR refuses
	wide := filepath.Join(root, "wide")
	for i := 0; i < 61; i++ {
		d := filepath.Join(wide, fmt.Sprintf("d%02d", i))
		os.MkdirAll(d, 0o755)
		for j := 0; j < 1000; j++ {
			if err := os.WriteFile(filepath.Join(d, fmt.Sprintf("f%d.txt", j)), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	os.WriteFile(filepath.Join(wide, "d60", "main.xo"), []byte(files["p/a/main.xo"]), 0o644)
	for _, c := range []string{"check", "fmt", "parse"} {
		code, got := xo(c, wide+"/...")
		if code == 0 || !strings.Contains(got, "the limit of a DIR/... walk") {
			t.Errorf("xo %s WIDE/...: exit %d:\n%s", c, code, got)
		}
	}
}
