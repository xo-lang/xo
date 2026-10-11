package rt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func completionApp() *cliCmd {
	add := &cliCmd{name: "add", help: "Adds widgets.\nMore text.", flags: []*cliFlag{
		{name: "count", short: "n", help: "how many", kind: cliInt, value: "int"},
		{name: "dry-run", help: "do nothing", kind: cliBool},
	}, args: []*cliArg{{name: "name"}}}
	remote := &cliCmd{name: "remote", help: "Manages remotes.", cmds: []*cliCmd{{name: "list", help: "Lists remotes."}}}
	return &cliCmd{name: "my-tool", version: "1.0", flags: []*cliFlag{
		{name: "verbose", short: "v", help: "print 'more'", kind: cliBool},
		{name: "format", help: "output format", kind: cliChoice, choices: []string{"text", "json"}, value: "text|json"},
		{name: "config", short: "c", help: "the file", kind: cliStr, value: "path"},
	}, cmds: []*cliCmd{add, remote}}
}

// TestCliBashCompletion sources the bash script and completes words the
// way bash's programmable completion calls it.
func TestCliBashCompletion(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	script := cliBash(completionApp())
	dir := t.TempDir()
	path := filepath.Join(dir, "c.bash")
	os.WriteFile(path, []byte(script), 0o644)
	cases := []struct {
		words []string
		want  string
	}{
		{[]string{"my-tool", ""}, "add remote --verbose -v --format --config -c --help -h --version"},
		{[]string{"my-tool", "--f"}, "--format"},
		{[]string{"my-tool", "--format", ""}, "text json"},
		{[]string{"my-tool", "--format", "j"}, "json"},
		{[]string{"my-tool", "a"}, "add"},
		{[]string{"my-tool", "-v", "add", "--"}, "--verbose --format --config --count --dry-run --help --version"},
		{[]string{"my-tool", "add", "--count", ""}, ""},
		{[]string{"my-tool", "remote", ""}, "list --verbose -v --format --config -c --help -h --version"},
		{[]string{"my-tool", "remote", "list", "-"}, "--verbose -v --format --config -c --help -h --version"},
	}
	for _, c := range cases {
		var sb strings.Builder
		sb.WriteString("source " + path + "\nCOMP_WORDS=(")
		for _, w := range c.words {
			sb.WriteString(" " + cliQ(w))
		}
		sb.WriteString(")\nCOMP_CWORD=" + strconv.Itoa(len(c.words)-1) + "\n_my_tool\necho \"${COMPREPLY[*]}\"\n")
		out, err := exec.Command(bash, "-c", sb.String()).CombinedOutput()
		if err != nil {
			t.Fatalf("bash: %v\n%s", err, out)
		}
		if got := strings.TrimSpace(string(out)); got != c.want {
			t.Errorf("%q: got %q, want %q", c.words, got, c.want)
		}
	}
}

// TestCliCompletionSyntax checks the zsh and fish scripts with the
// shells' syntax checkers, where installed.
func TestCliCompletionSyntax(t *testing.T) {
	app := completionApp()
	for _, sh := range []struct {
		name, script string
	}{{"bash", cliBash(app)}, {"zsh", cliZsh(app)}, {"fish", cliFish(app)}} {
		bin, err := exec.LookPath(sh.name)
		if err != nil {
			t.Logf("no %s: not checked", sh.name)
			continue
		}
		path := filepath.Join(t.TempDir(), "c."+sh.name)
		os.WriteFile(path, []byte(sh.script), 0o644)
		if out, err := exec.Command(bin, "-n", path).CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s\n%s", sh.name, err, out, sh.script)
		}
	}
}
