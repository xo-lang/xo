package twincheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestRepoConfigs: every twin directory in this checkout parses, so a
// malformed twin.txt, or a new twin with neither a twin.txt nor the default
// shape, fails here without building anything.
func TestRepoConfigs(t *testing.T) {
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) < 10 {
		t.Fatalf("found %d twin directories, fewer than the repo has", len(dirs))
	}
	for _, rel := range dirs {
		d, err := Load(root, rel)
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if d.Skip == "" && len(d.Cases) == 0 {
			t.Errorf("%s: no cases and no skip", rel)
		}
	}
}

// benchDir makes a bench directory with the files named (a name ending in
// "/" is a directory).
func benchDir(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "bench", "perf", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeConfig(t *testing.T, root, data string) {
	t.Helper()
	path := filepath.Join(root, "bench", "perf", "demo", ConfigName)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDefault: no twin.txt means one case of the directory's defaults.
func TestLoadDefault(t *testing.T) {
	root := benchDir(t, "main.xo", "twin.go.txt", "xo.mod")
	d, err := Load(root, "bench/perf/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Cases) != 1 || d.Cases[0].Xo != "main.xo" || d.Cases[0].Go != "twin.go.txt" {
		t.Fatalf("default case is %+v", d.Cases)
	}
	if d.Fields != 0 || d.Skip != "" || len(d.Env) != 0 {
		t.Fatalf("defaults: %+v", d)
	}
}

// TestLoadSubdirs: the xo/ and go/ shape.
func TestLoadSubdirs(t *testing.T) {
	root := benchDir(t, "xo/", "go/")
	d, err := Load(root, "bench/perf/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Cases) != 1 || d.Cases[0].Xo != "xo" || d.Cases[0].Go != "go" {
		t.Fatalf("default case is %+v", d.Cases)
	}
}

// TestParse: every line form, the defaults, and arguments to both sides or
// one.
func TestParse(t *testing.T) {
	root := benchDir(t, "main.xo", "twin.go.txt", "go/", "xo/")
	writeConfig(t, root, `
# how demo runs
env N=1000        # a small count
compare fields 2
main.xo - alpha beta
- go/ gamma
xo/ go -- mode 3
skip: a server, driven by run.sh
`)
	d, err := Load(root, "bench/perf/demo")
	if err != nil {
		t.Fatal(err)
	}
	if d.Fields != 2 || len(d.Env) != 1 || d.Env[0] != "N=1000" {
		t.Fatalf("directives: fields %d env %v", d.Fields, d.Env)
	}
	if d.Skip != "a server, driven by run.sh" {
		t.Fatalf("skip is %q", d.Skip)
	}
	want := []Case{
		{Xo: "main.xo", Go: "twin.go.txt", XoArgs: []string{"alpha", "beta"}, GoArgs: []string{"alpha", "beta"}},
		{Xo: "main.xo", Go: "go", XoArgs: []string{"gamma"}, GoArgs: []string{"gamma"}},
		{Xo: "xo", Go: "go", XoArgs: []string{}, GoArgs: []string{"mode", "3"}},
	}
	if len(d.Cases) != len(want) {
		t.Fatalf("cases:\n%+v", d.Cases)
	}
	for i := range want {
		got := d.Cases[i]
		if got.Xo != want[i].Xo || got.Go != want[i].Go ||
			strings.Join(got.XoArgs, " ") != strings.Join(want[i].XoArgs, " ") ||
			strings.Join(got.GoArgs, " ") != strings.Join(want[i].GoArgs, " ") {
			t.Errorf("case %d is %+v, want %+v", i, got, want[i])
		}
	}
}

// TestLoadErrors: a config that cannot be followed says why and where.
func TestLoadErrors(t *testing.T) {
	for _, c := range []struct{ config, want string }{
		{"main.xo\n", "a case is"},
		{"skip:\n", "skip needs a reason"},
		{"compare lines 2\n", "compare takes"},
		{"compare fields 0\n", "1 or more"},
		{"env N\n", "env takes KEY=VALUE"},
		{"# only comments\n", "no cases"},
	} {
		root := benchDir(t, "main.xo", "twin.go.txt")
		writeConfig(t, root, c.config)
		_, err := Load(root, "bench/perf/demo")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("config %q: error %v, want one mentioning %q", c.config, err, c.want)
		}
	}
}

// TestLoadNoTwin: a directory with no Go side cannot be loaded by the
// default rule.
func TestLoadNoTwin(t *testing.T) {
	root := benchDir(t, "main.xo")
	if _, err := Load(root, "bench/perf/demo"); err == nil {
		t.Fatal("a directory with no Go twin loaded")
	}
}

// TestCompare: the printed lines must agree, whole or by leading fields.
func TestCompare(t *testing.T) {
	for _, c := range []struct {
		xoOut, goOut string
		fields       int
		want         string
	}{
		{"cut 42\n", "cut 42\n", 0, ""},
		{"cut 42\n", "cut 42", 0, ""},
		{"cut 42\n", "cut 43\n", 0, "line 1 differs"},
		{"find 1\ncut 2\n", "find 1\ncut 2\ncut 3\n", 0, "2 lines from the Xo program, 3 from the Go twin"},
		{"lower 7 120\n", "lower 7 98\n", 2, ""},
		{"lower 7 120\n", "lower 8 120\n", 2, "line 1 differs"},
		{"lower 7 120\n", "lower 7 98\n", 0, "line 1 differs"},
		{"", "", 0, ""},
	} {
		got := Compare(c.xoOut, c.goOut, c.fields)
		if c.want == "" {
			if got != "" {
				t.Errorf("Compare(%q, %q, %d) = %q, want agreement", c.xoOut, c.goOut, c.fields, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("Compare(%q, %q, %d) = %q, want one mentioning %q", c.xoOut, c.goOut, c.fields, got, c.want)
		}
	}
}

// TestDiscover: bench/perf/NAME and bench/perf/GROUP/NAME with a twin; a
// program directory inside a bench (conc/xo/fanout) is not one.
func TestDiscover(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string, files ...string) {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "/") {
				if err := os.MkdirAll(filepath.Join(dir, f), 0o755); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("bench/perf/flat", "main.xo", "twin.go.txt")
	mk("bench/perf/grouped", "xo/", "go/")
	mk("bench/perf/mixed", "main.xo", "go/")
	mk("bench/perf/noTwin", "main.xo", "xo.mod")
	mk("bench/perf/grouped/xo/fanout", "main.xo", "xo.mod")
	mk("bench/perf/twins/hello", "xo/", "go/")
	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bench/perf/flat", "bench/perf/grouped", "bench/perf/mixed", "bench/perf/twins/hello"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Discover = %v, want %v", got, want)
	}
}

// TestCaseName: a case names its workload, and both sides only when they
// differ.
func TestCaseName(t *testing.T) {
	for _, c := range []struct {
		cs   Case
		want string
	}{
		{Case{}, ""},
		{Case{XoArgs: []string{"cut"}, GoArgs: []string{"cut"}}, "cut"},
		{Case{XoArgs: []string{"4"}, GoArgs: []string{"fanout", "4"}}, "4 / fanout 4"},
	} {
		if got := c.cs.Name(); got != c.want {
			t.Errorf("Name(%+v) = %q, want %q", c.cs, got, c.want)
		}
	}
}
