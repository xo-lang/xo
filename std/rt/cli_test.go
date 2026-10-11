package rt

import (
	"strings"
	"testing"
)

// cliFault runs f and returns the message of the fault it raises, or "".
func cliFault(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			if fe, ok := r.(*FaultErr); ok {
				msg = fe.Msg
				return
			}
			panic(r)
		}
	}()
	f()
	return ""
}

func cliCmdOf(name string, flags []*cliFlag, args []*cliArg, cmds ...*cliCmd) *cliCmd {
	return &cliCmd{name: name, flags: flags, args: args, cmds: cmds}
}

// TestCliValidate pins the faults of mistakes in a declaration (std.md
// 14.1): they are bugs in the program, reported when it parses.
func TestCliValidate(t *testing.T) {
	b := func(name, short string) *cliFlag { return &cliFlag{name: name, short: short, kind: cliBool} }
	cases := []struct {
		cmd     *cliCmd
		version bool
		want    string
	}{
		{cliCmdOf("tool", []*cliFlag{b("verbose", "v")}, nil), false, ""},
		{cliCmdOf("my tool", nil, nil), false, `cli: command name "my tool" must be letters, digits, ., _, and - (not first)`},
		{cliCmdOf("tool", []*cliFlag{b("Verbose", "")}, nil), false, `cli: flag name "Verbose" must be lowercase letters, digits, and - (command tool)`},
		{cliCmdOf("tool", []*cliFlag{b("-x", "")}, nil), false, `cli: flag name "-x" must be lowercase letters, digits, and - (command tool)`},
		{cliCmdOf("tool", []*cliFlag{b("help", "")}, nil), false, "cli: flag --help is reserved (command tool)"},
		{cliCmdOf("tool", []*cliFlag{b("version", "")}, nil), false, ""},
		{cliCmdOf("tool", []*cliFlag{b("version", "")}, nil), true, "cli: flag --version is reserved (command tool)"},
		{cliCmdOf("tool", []*cliFlag{b("all", "al")}, nil), false, `cli: short name "al" of --all must be one letter or digit`},
		{cliCmdOf("tool", []*cliFlag{b("hidden", "h")}, nil), false, "cli: short name -h of --hidden is reserved for --help"},
		{cliCmdOf("tool", []*cliFlag{b("a", ""), b("a", "")}, nil), false, "cli: flag --a is declared twice (command tool)"},
		{cliCmdOf("tool", []*cliFlag{b("a", "x"), b("b", "x")}, nil), false, "cli: short name -x is used by --a and --b"},
		{cliCmdOf("tool", []*cliFlag{b("a", "")}, nil, cliCmdOf("sub", []*cliFlag{b("a", "")}, nil)), false, "cli: flag --a is declared twice (command sub)"},
		{cliCmdOf("tool", nil, nil, cliCmdOf("x", []*cliFlag{b("a", "")}, nil), cliCmdOf("y", []*cliFlag{b("a", "")}, nil)), false, ""},
		{cliCmdOf("tool", []*cliFlag{{name: "fmt", kind: cliChoice}}, nil), false, "cli: choice flag --fmt has no choices"},
		{cliCmdOf("tool", []*cliFlag{{name: "fmt", kind: cliChoice, choices: []string{"a"}, defStr: "b"}}, nil), false, `cli: default "b" of --fmt is not one of its choices`},
		{cliCmdOf("tool", nil, []*cliArg{{name: "x"}}, cliCmdOf("sub", nil, nil)), false, "cli: command tool has both positional arguments and subcommands"},
		{cliCmdOf("tool", nil, []*cliArg{{name: ""}}), false, "cli: an argument of command tool has no name"},
		{cliCmdOf("tool", nil, []*cliArg{{name: "x"}, {name: "x"}}), false, "cli: argument <x> is declared twice (command tool)"},
		{cliCmdOf("tool", nil, []*cliArg{{name: "xs", rest: true}, {name: "y"}}), false, "cli: rest argument <xs> must be the last argument (command tool)"},
		{cliCmdOf("tool", nil, []*cliArg{{name: "x", optional: true}, {name: "y"}}), false, "cli: required argument <y> follows an optional one (command tool)"},
		{cliCmdOf("tool", nil, nil, cliCmdOf("a", nil, nil), cliCmdOf("a", nil, nil)), false, "cli: command a is declared twice under tool"},
	}
	for _, c := range cases {
		got := cliFault(func() { cliValidate(c.cmd, nil, c.version) })
		if got != c.want {
			t.Errorf("%s: fault %q, want %q", c.cmd.name, got, c.want)
		}
	}
}

// TestCliSuggest pins the did you mean rule: at most two edits, fewer
// than the length of the word, the first candidate on a tie.
func TestCliSuggest(t *testing.T) {
	cands := []string{"count", "config", "color", "help"}
	for _, c := range []struct{ in, want string }{
		{"cont", "count"},
		{"confg", "config"},
		{"conf", "count"}, // two edits from count and from config: the first wins
		{"colour", "color"},
		{"hlep", "help"},
		{"x", ""},
		{"zzzzzz", ""},
		{"co", ""},
	} {
		if got := cliSuggest(c.in, cands); got != c.want {
			t.Errorf("suggest %q: %q, want %q", c.in, got, c.want)
		}
	}
	if d := cliDistance("kitten", "sitting"); d != 3 {
		t.Errorf("distance kitten sitting: %d, want 3", d)
	}
}

// TestCliHelpAlignment checks that every row of a help text starts its
// description in the same column.
func TestCliHelpAlignment(t *testing.T) {
	root := cliCmdOf("tool", []*cliFlag{
		{name: "verbose", short: "v", help: "more", kind: cliBool},
		{name: "output-directory", help: "where", kind: cliStr, value: "dir", defStr: "out"},
	}, []*cliArg{{name: "file", help: "input"}})
	root.version = "1.0"
	text := cliHelpText([]*cliCmd{root})
	col := -1
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "  ") {
			continue
		}
		// descriptions hold no double space: the last one ends the padding
		c := strings.LastIndex(l, "  ") + 2
		if col < 0 {
			col = c
		} else if c != col {
			t.Errorf("row %q starts its description at %d, want %d\n%s", l, c, col, text)
		}
	}
	if !strings.Contains(text, "--output-directory <dir>  where [default: out]") {
		t.Errorf("help text:\n%s", text)
	}
}
