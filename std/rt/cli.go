package rt

import (
	"strconv"
	"strings"
)

// std/cli (std.md 14, decision 0160): command line parsing. The native
// backends implement the same rules in Xo (std/native/cli.xo); the help
// text, usage errors, and faults must stay identical, byte for byte
// (the golden programs cli_* run on both).

const (
	cliBool = iota
	cliStr
	cliInt
	cliDur
	cliChoice
	cliStrs
	cliFloat
)

var cliKindName = [...]string{"bool", "str", "int", "duration", "choice", "strs", "float"}

// cliReaders names the accessors of each kind of flag.
var cliReaders = [...]string{"bool", "str or str_opt", "int or int_opt", "duration or duration_opt", "str or str_opt", "strs", "float or float_opt"}

type cliFlag struct {
	name, short, help, env string
	kind                   int
	defStr                 string
	defInt                 int64
	defDur                 Duration
	defFloat               float64
	required               bool
	value                  string
	choices                []string
}

type cliArg struct {
	name, help string
	optional   bool
	rest       bool
	min        int64
}

type cliCmd struct {
	name, help, version string
	flags               []*cliFlag
	args                []*cliArg
	cmds                []*cliCmd
}

// Cli_Command is `cli.Command`.
type Cli_Command struct{ c *cliCmd }

// Cli_Flag is `cli.Flag`.
type Cli_Flag struct{ f *cliFlag }

// Cli_Arg is `cli.Arg`.
type Cli_Arg struct{ a *cliArg }

// Cli_Parsed is `cli.Parsed`.
type Cli_Parsed struct{ p *cliParsed }

type cliVal struct {
	set bool
	b   bool
	s   string
	i   int64
	d   Duration
	f   float64
	l   []string
}

type cliParsed struct {
	root    *cliCmd
	path    []*cliCmd // root first
	vals    map[*cliFlag]*cliVal
	argVals map[*cliArg][]string
}

// XoDebug: the handles show as `<command>`, `<flag>`, `<arg>`, and
// `<parsed>`, the native backends' text of an opaque std handle.
func (Cli_Command) XoDebug(b *strings.Builder) { b.WriteString("<command>") }
func (Cli_Flag) XoDebug(b *strings.Builder)    { b.WriteString("<flag>") }
func (Cli_Arg) XoDebug(b *strings.Builder)     { b.WriteString("<arg>") }
func (Cli_Parsed) XoDebug(b *strings.Builder)  { b.WriteString("<parsed>") }

func (c Cli_Command) M_debug(*Task) string { return Debug(c) }
func (f Cli_Flag) M_debug(*Task) string    { return Debug(f) }
func (a Cli_Arg) M_debug(*Task) string     { return Debug(a) }
func (p Cli_Parsed) M_debug(*Task) string  { return Debug(p) }

// M_display is the help or version text, or the usage message.
func (e Cli_CliErr) M_display(*Task) string {
	switch p := e.P.(type) {
	case Cli_CliErr_Help:
		return p.F_text
	case Cli_CliErr_Version:
		return p.F_text
	case Cli_CliErr_Usage:
		return p.F_msg
	}
	return ""
}

// M_exit_code is 0 for Help and Version, 2 for Usage.
func (e Cli_CliErr) M_exit_code(*Task) int64 {
	if _, ok := e.P.(Cli_CliErr_Usage); ok {
		return 2
	}
	return 0
}

func cliHelpErr(text string) Cli_CliErr {
	return Cli_CliErr{Tag: 1, P: Cli_CliErr_Help{F_text: text}}
}

func cliVersionErr(text string) Cli_CliErr {
	return Cli_CliErr{Tag: 2, P: Cli_CliErr_Version{F_text: text}}
}

// Cli_command is `cli.command(...)`.
func Cli_command(_ *Task, name, help string, flags List[Cli_Flag], args List[Cli_Arg], commands List[Cli_Command], version string) Cli_Command {
	c := &cliCmd{name: name, help: help, version: version}
	for _, f := range flags.s {
		c.flags = append(c.flags, f.f)
	}
	for _, a := range args.s {
		c.args = append(c.args, a.a)
	}
	for _, s := range commands.s {
		c.cmds = append(c.cmds, s.c)
	}
	return Cli_Command{c}
}

// Cli_bool_flag is `cli.bool_flag(...)`.
func Cli_bool_flag(_ *Task, name, help, short, env string) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliBool}}
}

// Cli_str_flag is `cli.str_flag(...)`.
func Cli_str_flag(_ *Task, name, help, short, def, env string, required bool, value string) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliStr, defStr: def, required: required, value: value}}
}

// Cli_int_flag is `cli.int_flag(...)`.
func Cli_int_flag(_ *Task, name, help, short string, def int64, env string, required bool) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliInt, defInt: def, required: required, value: "int"}}
}

// Cli_duration_flag is `cli.duration_flag(...)`.
func Cli_duration_flag(_ *Task, name, help, short string, def Duration, env string, required bool) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliDur, defDur: def, required: required, value: "duration"}}
}

// Cli_float_flag is `cli.float_flag(...)`.
func Cli_float_flag(_ *Task, name, help, short string, def float64, env string, required bool) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliFloat, defFloat: def, required: required, value: "float"}}
}

// Cli_choice_flag is `cli.choice_flag(...)`.
func Cli_choice_flag(_ *Task, name, help string, choices List[string], short, def, env string, required bool) Cli_Flag {
	cs := append([]string(nil), choices.s...)
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliChoice, defStr: def, required: required, value: strings.Join(cs, "|"), choices: cs}}
}

// Cli_strs_flag is `cli.strs_flag(...)`.
func Cli_strs_flag(_ *Task, name, help, short, env, value string) Cli_Flag {
	return Cli_Flag{&cliFlag{name: name, short: short, help: help, env: env, kind: cliStrs, value: value}}
}

// Cli_arg is `cli.arg(...)`.
func Cli_arg(_ *Task, name, help string, optional bool) Cli_Arg {
	return Cli_Arg{&cliArg{name: name, help: help, optional: optional}}
}

// Cli_rest is `cli.rest(...)`.
func Cli_rest(_ *Task, name, help string, min int64) Cli_Arg {
	return Cli_Arg{&cliArg{name: name, help: help, rest: true, min: min}}
}

// ---------------------------------------------------------------------------
// Validation: mistakes in the declaration are faults.

func cliFlagName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && c == '-'
		if !ok {
			return false
		}
	}
	return true
}

func cliShortName(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// cliCmdName: letters, digits, `.`, `_`, and `-` (not first), so a name
// needs no quoting in shells and completion scripts (decision 0165).
func cliCmdName(s string) bool {
	if s == "" || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// cliValidate faults on the first mistake in the tree under c; outer
// holds the flags of the commands above c.
func cliValidate(c *cliCmd, outer []*cliFlag, version bool) {
	if !cliCmdName(c.name) {
		FaultHere("cli: command name %s must be letters, digits, ., _, and - (not first)", strconv.Quote(c.name))
	}
	seen := append([]*cliFlag(nil), outer...)
	for _, f := range c.flags {
		if !cliFlagName(f.name) {
			FaultHere("cli: flag name %s must be lowercase letters, digits, and - (command %s)", strconv.Quote(f.name), c.name)
		}
		if f.name == "help" || version && f.name == "version" {
			FaultHere("cli: flag --%s is reserved (command %s)", f.name, c.name)
		}
		if f.short != "" && !cliShortName(f.short) {
			FaultHere("cli: short name %s of --%s must be one letter or digit", strconv.Quote(f.short), f.name)
		}
		if f.short == "h" {
			FaultHere("cli: short name -h of --%s is reserved for --help", f.name)
		}
		for _, o := range seen {
			if o.name == f.name {
				FaultHere("cli: flag --%s is declared twice (command %s)", f.name, c.name)
			}
			if f.short != "" && o.short == f.short {
				FaultHere("cli: short name -%s is used by --%s and --%s", f.short, o.name, f.name)
			}
		}
		if f.kind == cliChoice {
			if len(f.choices) == 0 {
				FaultHere("cli: choice flag --%s has no choices", f.name)
			}
			if f.defStr != "" && !cliContains(f.choices, f.defStr) {
				FaultHere("cli: default %s of --%s is not one of its choices", strconv.Quote(f.defStr), f.name)
			}
		}
		seen = append(seen, f)
	}
	if len(c.args) > 0 && len(c.cmds) > 0 {
		FaultHere("cli: command %s has both positional arguments and subcommands", c.name)
	}
	optional := false
	for i, a := range c.args {
		if a.name == "" {
			FaultHere("cli: an argument of command %s has no name", c.name)
		}
		for _, o := range c.args[:i] {
			if o.name == a.name {
				FaultHere("cli: argument <%s> is declared twice (command %s)", a.name, c.name)
			}
		}
		if a.rest && i != len(c.args)-1 {
			FaultHere("cli: rest argument <%s> must be the last argument (command %s)", a.name, c.name)
		}
		if !a.rest && !a.optional && optional {
			FaultHere("cli: required argument <%s> follows an optional one (command %s)", a.name, c.name)
		}
		if a.optional || a.rest {
			optional = true
		}
	}
	for i, s := range c.cmds {
		for _, o := range c.cmds[:i] {
			if o.name == s.name {
				FaultHere("cli: command %s is declared twice under %s", s.name, c.name)
			}
		}
		cliValidate(s, seen, version)
	}
}

func cliContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Help and usage text

func cliPathName(path []*cliCmd) string {
	var ws []string
	for _, c := range path {
		ws = append(ws, c.name)
	}
	return strings.Join(ws, " ")
}

func cliArgLabel(a *cliArg) string {
	switch {
	case a.rest && a.min > 0:
		return "<" + a.name + ">..."
	case a.rest:
		return "[<" + a.name + ">...]"
	case a.optional:
		return "[<" + a.name + ">]"
	}
	return "<" + a.name + ">"
}

func cliUsageLine(path []*cliCmd) string {
	c := path[len(path)-1]
	s := "Usage: " + cliPathName(path) + " [flags]"
	if len(c.cmds) > 0 {
		return s + " <command>"
	}
	for _, a := range c.args {
		s += " " + cliArgLabel(a)
	}
	return s
}

func cliUsageText(path []*cliCmd) string {
	return cliUsageLine(path) + "\n\nRun '" + cliPathName(path) + " --help' for more information."
}

func cliFlagLeft(f *cliFlag) string {
	s := "    --" + f.name
	if f.short != "" {
		s = "-" + f.short + ", --" + f.name
	}
	if f.kind != cliBool {
		s += " <" + f.value + ">"
	}
	return s
}

func cliFlagHelp(f *cliFlag) string {
	s := f.help
	add := func(x string) {
		if s == "" {
			s = x
		} else {
			s += " " + x
		}
	}
	switch f.kind {
	case cliStr, cliChoice:
		if f.defStr != "" {
			add("[default: " + f.defStr + "]")
		}
	case cliInt:
		if f.defInt != 0 {
			add("[default: " + strconv.FormatInt(f.defInt, 10) + "]")
		}
	case cliDur:
		if f.defDur != 0 {
			add("[default: " + f.defDur.String() + "]")
		}
	case cliFloat:
		if f.defFloat != 0 {
			add("[default: " + FloatStr(f.defFloat) + "]")
		}
	}
	if f.env != "" {
		add("[env: " + f.env + "]")
	}
	if f.required {
		add("[required]")
	}
	if f.kind == cliStrs {
		add("[repeatable]")
	}
	return s
}

type cliRow struct{ left, right string }

func cliHelpText(path []*cliCmd) string {
	root, c := path[0], path[len(path)-1]
	type section struct {
		title string
		rows  []cliRow
	}
	var secs []section
	if len(c.cmds) > 0 {
		var rows []cliRow
		for _, s := range c.cmds {
			rows = append(rows, cliRow{s.name, s.help})
		}
		secs = append(secs, section{"Commands:", rows})
	}
	if len(c.args) > 0 {
		var rows []cliRow
		for _, a := range c.args {
			rows = append(rows, cliRow{cliArgLabel(a), a.help})
		}
		secs = append(secs, section{"Arguments:", rows})
	}
	var rows []cliRow
	for _, f := range c.flags {
		rows = append(rows, cliRow{cliFlagLeft(f), cliFlagHelp(f)})
	}
	rows = append(rows, cliRow{"-h, --help", "show this help"})
	if root.version != "" {
		rows = append(rows, cliRow{"    --version", "show the version"})
	}
	secs = append(secs, section{"Flags:", rows})
	var global []cliRow
	for _, p := range path[:len(path)-1] {
		for _, f := range p.flags {
			global = append(global, cliRow{cliFlagLeft(f), cliFlagHelp(f)})
		}
	}
	if len(global) > 0 {
		secs = append(secs, section{"Global flags:", global})
	}
	w := 0
	for _, s := range secs {
		for _, r := range s.rows {
			if len(r.left) > w {
				w = len(r.left)
			}
		}
	}
	var b strings.Builder
	if c.help != "" {
		b.WriteString(c.help + "\n\n")
	}
	b.WriteString(cliUsageLine(path))
	for _, s := range secs {
		b.WriteString("\n\n" + s.title)
		for _, r := range s.rows {
			b.WriteString("\n  " + r.left)
			if r.right != "" {
				b.WriteString(strings.Repeat(" ", w-len(r.left)+2) + r.right)
			}
		}
	}
	return b.String()
}

// Cli_help is `cli.help(cmd, path)`.
func Cli_help(_ *Task, cmd Cli_Command, path string) string {
	root := cmd.c
	cliValidate(root, nil, root.version != "")
	p := []*cliCmd{root}
	if path != "" {
		for _, w := range strings.Split(path, " ") {
			cur := p[len(p)-1]
			var next *cliCmd
			for _, s := range cur.cmds {
				if s.name == w {
					next = s
					break
				}
			}
			if next == nil {
				FaultHere("cli: no command %s in %s", strconv.Quote(path), root.name)
			}
			p = append(p, next)
		}
	}
	return cliHelpText(p)
}

// ---------------------------------------------------------------------------
// Parsing

func cliDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// cliSuggest is the candidate closest to s (at most 2 edits, fewer than
// the length of s), the first one on a tie, or "".
func cliSuggest(s string, cands []string) string {
	best, bd := "", 3
	for _, c := range cands {
		d := cliDistance(s, c)
		if d < bd && d < len(s) {
			best, bd = c, d
		}
	}
	return best
}

type cliUsageErr struct{ msg string }

func cliValueErr(f *cliFlag, v, env, want string) *cliUsageErr {
	msg := "invalid value " + strconv.Quote(v) + " for --" + f.name
	if env != "" {
		msg += " (from $" + env + ")"
	}
	return &cliUsageErr{msg + ": expected " + want}
}

// cliSet records value v for f (env is the variable it came from, or "").
func cliSet(vals map[*cliFlag]*cliVal, f *cliFlag, v, env string) *cliUsageErr {
	x := vals[f]
	switch f.kind {
	case cliBool:
		switch v {
		case "true", "1":
			x.b = true
		case "false", "0":
			x.b = false
		default:
			return cliValueErr(f, v, env, "true or false")
		}
	case cliStr:
		x.s = v
	case cliChoice:
		if !cliContains(f.choices, v) {
			return cliValueErr(f, v, env, "one of "+strings.Join(f.choices, ", "))
		}
		x.s = v
	case cliInt:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return cliValueErr(f, v, env, "an integer")
		}
		x.i = n
	case cliDur:
		d, ok := parseDuration(v)
		if !ok {
			return cliValueErr(f, v, env, "a duration such as 30s or 1m30s")
		}
		x.d = d
	case cliFloat:
		n := Float_parse(nil, v)
		if !n.Some {
			return cliValueErr(f, v, env, "a number")
		}
		x.f = n.V
	case cliStrs:
		if env != "" {
			x.l = append(x.l, strings.Split(v, ",")...)
		} else {
			x.l = append(x.l, v)
		}
	}
	x.set = true
	return nil
}

func cliParse(tk *Task, root *cliCmd, args []string, env Env) (*cliParsed, *Cli_CliErr) {
	version := root.version != ""
	cliValidate(root, nil, version)
	p := &cliParsed{root: root, path: []*cliCmd{root}, vals: map[*cliFlag]*cliVal{}, argVals: map[*cliArg][]string{}}
	var active []*cliFlag
	addFlags := func(c *cliCmd) {
		for _, f := range c.flags {
			active = append(active, f)
			p.vals[f] = &cliVal{}
		}
	}
	addFlags(root)
	usage := func(msg string) (*cliParsed, *Cli_CliErr) {
		e := Cli_CliErr{Tag: 3, P: Cli_CliErr_Usage{F_msg: msg, F_usage: cliUsageText(p.path)}}
		return nil, &e
	}
	help := func() (*cliParsed, *Cli_CliErr) {
		e := cliHelpErr(cliHelpText(p.path))
		return nil, &e
	}
	var pos []string
	dashes := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		cur := p.path[len(p.path)-1]
		switch {
		case dashes:
			pos = append(pos, a)
			continue
		case a == "--":
			dashes = true
			continue
		case strings.HasPrefix(a, "--"):
			name, val, has := strings.Cut(a[2:], "=")
			if name == "help" {
				return help()
			}
			if version && name == "version" {
				e := cliVersionErr(root.name + " " + root.version)
				return nil, &e
			}
			var f *cliFlag
			for _, x := range active {
				if x.name == name {
					f = x
				}
			}
			if f == nil {
				cands := []string{}
				for _, x := range active {
					cands = append(cands, x.name)
				}
				cands = append(cands, "help")
				if version {
					cands = append(cands, "version")
				}
				msg := "unknown flag --" + name
				if s := cliSuggest(name, cands); s != "" {
					msg += " (did you mean --" + s + "?)"
				}
				return usage(msg)
			}
			if f.kind == cliBool && !has {
				val = "true"
			} else if !has {
				if i+1 >= len(args) {
					return usage("flag --" + f.name + " needs a value")
				}
				i++
				val = args[i]
			}
			if e := cliSet(p.vals, f, val, ""); e != nil {
				return usage(e.msg)
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			cl := a[1:]
			for j := 0; j < len(cl); j++ {
				c := cl[j : j+1]
				if c == "h" {
					return help()
				}
				var f *cliFlag
				for _, x := range active {
					if x.short == c {
						f = x
					}
				}
				if f == nil {
					return usage("unknown flag -" + c)
				}
				if f.kind == cliBool {
					if e := cliSet(p.vals, f, "true", ""); e != nil {
						return usage(e.msg)
					}
					continue
				}
				val := strings.TrimPrefix(cl[j+1:], "=")
				if j+1 >= len(cl) {
					if i+1 >= len(args) {
						return usage("flag --" + f.name + " needs a value")
					}
					i++
					val = args[i]
				}
				if e := cliSet(p.vals, f, val, ""); e != nil {
					return usage(e.msg)
				}
				break
			}
		default:
			if len(cur.cmds) > 0 {
				var next *cliCmd
				var names []string
				for _, s := range cur.cmds {
					names = append(names, s.name)
					if s.name == a {
						next = s
					}
				}
				if next == nil {
					msg := "unknown command " + strconv.Quote(a)
					if s := cliSuggest(a, names); s != "" {
						msg += " (did you mean " + s + "?)"
					}
					return usage(msg)
				}
				p.path = append(p.path, next)
				addFlags(next)
				continue
			}
			pos = append(pos, a)
		}
	}
	cur := p.path[len(p.path)-1]
	if len(cur.cmds) > 0 {
		var names []string
		for _, s := range cur.cmds {
			names = append(names, s.name)
		}
		return usage("missing command (one of " + strings.Join(names, ", ") + ")")
	}
	for _, f := range active {
		x := p.vals[f]
		if !x.set && f.env != "" {
			if v := env.M_get(tk, f.env); v.Some && v.V != "" {
				if e := cliSet(p.vals, f, v.V, f.env); e != nil {
					return usage(e.msg)
				}
			}
		}
		if !x.set && f.required {
			return usage("missing required flag --" + f.name)
		}
	}
	k := 0
	for _, a := range cur.args {
		switch {
		case a.rest:
			rest := append([]string{}, pos[k:]...)
			if int64(len(rest)) < a.min {
				return usage("missing argument <" + a.name + ">")
			}
			p.argVals[a] = rest
			k = len(pos)
		case k < len(pos):
			p.argVals[a] = []string{pos[k]}
			k++
		case !a.optional:
			return usage("missing argument <" + a.name + ">")
		}
	}
	if k < len(pos) {
		return usage("unexpected argument " + strconv.Quote(pos[k]))
	}
	return p, nil
}

// Cli_parse is `cli.parse(cmd, args, env)`.
func Cli_parse(tk *Task, cmd Cli_Command, args List[string], env Env) Result[Cli_Parsed, Cli_CliErr] {
	p, e := cliParse(tk, cmd.c, args.s, env)
	if e != nil {
		return Err[Cli_Parsed](*e)
	}
	return Ok[Cli_Parsed, Cli_CliErr](Cli_Parsed{p})
}

// Cli_parse_or_exit is `cli.parse_or_exit(cmd, env, stdio, proc)`.
func Cli_parse_or_exit(tk *Task, cmd Cli_Command, env Env, stdio Stdio, proc Proc) Cli_Parsed {
	p, e := cliParse(tk, cmd.c, env.M_args(tk).s, env)
	if e == nil {
		return Cli_Parsed{p}
	}
	switch x := e.P.(type) {
	case Cli_CliErr_Help:
		stdio.M_println(tk, x.F_text)
		proc.M_exit(tk, 0)
	case Cli_CliErr_Version:
		stdio.M_println(tk, x.F_text)
		proc.M_exit(tk, 0)
	case Cli_CliErr_Usage:
		stdio.M_eprintln(tk, "error: "+x.F_msg)
		stdio.M_eprintln(tk, "")
		stdio.M_eprintln(tk, x.F_usage)
		proc.M_exit(tk, 2)
	}
	panic("unreachable")
}

// ---------------------------------------------------------------------------
// Accessors

func (p *cliParsed) command() string {
	var ws []string
	for _, c := range p.path[1:] {
		ws = append(ws, c.name)
	}
	return strings.Join(ws, " ")
}

// flag finds the flag name: on the selected path, else anywhere in the
// tree (a flag of a command that was not selected is unset).
func (p *cliParsed) flag(name string, kinds ...int) (*cliFlag, *cliVal) {
	var f *cliFlag
	for _, c := range p.path {
		for _, x := range c.flags {
			if x.name == name {
				f = x
			}
		}
	}
	if f == nil {
		f = cliFindFlag(p.root, name)
	}
	if f == nil {
		FaultHere("cli: flag --%s is not declared", name)
	}
	ok := false
	for _, k := range kinds {
		if f.kind == k {
			ok = true
		}
	}
	if !ok {
		FaultHere("cli: --%s is declared by %s_flag, so read it with %s", name, cliKindName[f.kind], cliReaders[f.kind])
	}
	if v := p.vals[f]; v != nil {
		return f, v
	}
	return f, &cliVal{}
}

func cliFindFlag(c *cliCmd, name string) *cliFlag {
	for _, f := range c.flags {
		if f.name == name {
			return f
		}
	}
	for _, s := range c.cmds {
		if f := cliFindFlag(s, name); f != nil {
			return f
		}
	}
	return nil
}

// arg finds the positional argument name: of the selected command, else
// anywhere in the tree (with no value).
func (p *cliParsed) arg(name string) (*cliArg, []string, bool) {
	cur := p.path[len(p.path)-1]
	for _, a := range cur.args {
		if a.name == name {
			v, ok := p.argVals[a]
			return a, v, ok
		}
	}
	if a := cliFindArg(p.root, name); a != nil {
		return a, nil, false
	}
	FaultHere("cli: argument <%s> is not declared", name)
	return nil, nil, false
}

func cliFindArg(c *cliCmd, name string) *cliArg {
	for _, a := range c.args {
		if a.name == name {
			return a
		}
	}
	for _, s := range c.cmds {
		if a := cliFindArg(s, name); a != nil {
			return a
		}
	}
	return nil
}

func (p Cli_Parsed) M_command(*Task) string { return p.p.command() }

func (p Cli_Parsed) M_bool(_ *Task, name string) bool {
	_, v := p.p.flag(name, cliBool)
	return v.b
}

func (p Cli_Parsed) M_str(_ *Task, name string) string {
	f, v := p.p.flag(name, cliStr, cliChoice)
	if v.set {
		return v.s
	}
	return f.defStr
}

func (p Cli_Parsed) M_str_opt(_ *Task, name string) Option[string] {
	_, v := p.p.flag(name, cliStr, cliChoice)
	if v.set {
		return Some(v.s)
	}
	return None[string]()
}

func (p Cli_Parsed) M_int(_ *Task, name string) int64 {
	f, v := p.p.flag(name, cliInt)
	if v.set {
		return v.i
	}
	return f.defInt
}

func (p Cli_Parsed) M_int_opt(_ *Task, name string) Option[int64] {
	_, v := p.p.flag(name, cliInt)
	if v.set {
		return Some(v.i)
	}
	return None[int64]()
}

func (p Cli_Parsed) M_duration(_ *Task, name string) Duration {
	f, v := p.p.flag(name, cliDur)
	if v.set {
		return v.d
	}
	return f.defDur
}

func (p Cli_Parsed) M_duration_opt(_ *Task, name string) Option[Duration] {
	_, v := p.p.flag(name, cliDur)
	if v.set {
		return Some(v.d)
	}
	return None[Duration]()
}

func (p Cli_Parsed) M_float(_ *Task, name string) float64 {
	f, v := p.p.flag(name, cliFloat)
	if v.set {
		return v.f
	}
	return f.defFloat
}

func (p Cli_Parsed) M_float_opt(_ *Task, name string) Option[float64] {
	_, v := p.p.flag(name, cliFloat)
	if v.set {
		return Some(v.f)
	}
	return None[float64]()
}

func (p Cli_Parsed) M_strs(_ *Task, name string) List[string] {
	_, v := p.p.flag(name, cliStrs)
	return ListOf(v.l...)
}

func (p Cli_Parsed) M_is_set(_ *Task, name string) bool {
	_, v := p.p.flag(name, cliBool, cliStr, cliInt, cliDur, cliChoice, cliStrs, cliFloat)
	return v.set
}

func (p Cli_Parsed) M_arg(_ *Task, name string) string {
	a, v, ok := p.p.arg(name)
	if a.rest {
		FaultHere("cli: <%s> is a rest argument; read it with rest", name)
	}
	if a.optional {
		FaultHere("cli: <%s> is optional; read it with arg_opt", name)
	}
	if !ok {
		FaultHere("cli: argument <%s> belongs to a command that was not selected", name)
	}
	return v[0]
}

func (p Cli_Parsed) M_arg_opt(_ *Task, name string) Option[string] {
	a, v, ok := p.p.arg(name)
	if a.rest {
		FaultHere("cli: <%s> is a rest argument; read it with rest", name)
	}
	if !ok {
		return None[string]()
	}
	return Some(v[0])
}

func (p Cli_Parsed) M_rest(_ *Task, name string) List[string] {
	a, v, _ := p.p.arg(name)
	if !a.rest {
		FaultHere("cli: <%s> is not a rest argument; read it with arg or arg_opt", name)
	}
	return ListOf(v...)
}
