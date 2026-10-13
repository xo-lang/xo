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
// selfhost/cli/cmdhelp lists them.
var helpCommands = []string{"build", "run", "test", "check", "parse", "fmt", "fix", "vet", "query", "edit", "doc", "explain", "init", "get", "mod", "vendor", "publish", "audit", "bind", "dump", "replay", "lsp", "mcp", "serve", "version"}

// shippedXo is the shipped xo (xo1, decision 0178 switch point B) whose
// command line the help tests drive: $XO_CLI, else, without -short,
// selfhost/xo built with the Go backend (about 20 s).
func shippedXo(t *testing.T) []string {
	t.Helper()
	if cli := strings.Fields(os.Getenv(cliEnv)); len(cli) > 0 {
		return cli
	}
	if testing.Short() {
		t.Skip("builds xo1 (selfhost/xo); set XO_CLI to run it with -short")
	}
	bin := filepath.Join(t.TempDir(), "xo")
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	var out, errOut bytes.Buffer
	if code := runXo([]string{"build", "-o", bin, filepath.Join("..", "..", "selfhost", "xo")}, &out, &errOut); code != 0 {
		t.Fatalf("building xo1: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	return []string{bin}
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
	cli := shippedXo(t)
	t.Chdir(t.TempDir())
	xo := func(args ...string) (string, string, int) {
		var out, errOut bytes.Buffer
		code := execCLI(cli, args, &out, &errOut)
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

// TestCLIPattern checks the pattern DIR/... of the shipped xo (decision
// 0210, spec/core.md 10.1): every command that takes directories reads
// DIR/... as DIR, which already covers the directories below it, so the
// two print the same; directories named testdata or starting with . or _
// are skipped; a DIR/... with no .xo file at or below DIR is an error
// (exit 1).
func TestCLIPattern(t *testing.T) {
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
	t.Chdir(filepath.Join(root, "m"))
	t.Setenv("XO_BUILD_DIR", filepath.Join(root, "build"))
	xo := func(args ...string) string {
		var out, errOut bytes.Buffer
		code := execCLI(cli, args, &out, &errOut)
		return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", code, out.String(), errOut.String())
	}
	for _, c := range [][]string{
		{"check", "."}, {"check", "sub"}, {"check", "--json", "broken"}, {"vet", "sub"}, {"doc", "sub"},
		{"fmt", "--check", "sub"}, {"fix", "--dry-run", "sub"}, {"parse", "sub"}, {"test", "sub"},
		{"build", "-o", filepath.Join(root, "bin"), "sub/.."},
	} {
		plain := xo(c...)
		pat := append(append([]string{}, c[:len(c)-1]...), c[len(c)-1]+"/...")
		if got := xo(pat...); got != plain {
			t.Errorf("xo %s:\n%s\nwant (xo %s):\n%s", strings.Join(pat, " "), got, strings.Join(c, " "), plain)
		}
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
