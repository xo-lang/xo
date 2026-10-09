package check

// Compile time checks of std/time layouts (std.md 13.3, decision 0094).
// A layout argument of time.format, time.parse, or time.check_layout that
// is a constant (a string literal, a const, or a `+` of them) is parsed
// here with the rules of the runtime (std/rt/timefmt.go; the test
// TestTimeLayoutAgrees keeps the two parsers equal):
//
//   - date patterns of other languages, `{yyyy}`, `yyyy-MM-dd`, `%Y`, and
//     Go's reference date `2006-01-02`, are the habit XO0177 with a
//     machine fix to the named fields when every piece has one;
//   - any other error (an unknown field, a stray brace) is XO0420, with a
//     did you mean fix for a misspelled field.

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// ConstString is the value of e when it is a compile time string: a
// literal without interpolation, a constant, or a `+` of them.
func ConstString(info *Info, e ast.Expr) (string, bool) {
	return info.c.constStr(e, 0)
}

func (c *checker) constStr(e ast.Expr, depth int) (string, bool) {
	if depth > 32 {
		return "", false
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return c.constStr(x.X, depth+1)
	case *ast.StringLit:
		var b strings.Builder
		for _, p := range x.Parts {
			if p.Expr != nil {
				return "", false
			}
			b.WriteString(p.Text)
		}
		return b.String(), true
	case *ast.BinaryExpr:
		if x.Op != token.Plus {
			return "", false
		}
		a, ok := c.constStr(x.X, depth+1)
		if !ok {
			return "", false
		}
		b, ok := c.constStr(x.Y, depth+1)
		return a + b, ok
	case *ast.Ident, *ast.SelectorExpr:
		if co, ok := c.rec.ref[e].(*ConstObj); ok && co.Decl != nil && co.Decl.Value != nil {
			return c.constStr(co.Decl.Value, depth+1)
		}
	}
	return "", false
}

// timeLayoutParams maps the std/time functions that take a layout to the
// index of the layout parameter.
var timeLayoutParams = map[string]int{"format": 2, "parse": 1, "check_layout": 0}

// checkTimeLayouts checks every constant layout passed to std/time.
func (c *checker) checkTimeLayouts() {
	tm := c.modules["std/time"]
	if tm == nil {
		return
	}
	for _, m := range c.order {
		if m.IsStd {
			continue
		}
		for _, f := range m.Files {
			c.file = f
			inspect(f.AST, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ci := c.callInfo[call]
				if ci == nil || ci.fn == nil || ci.fn.Module != tm || ci.fn.Recv != nil {
					return true
				}
				idx, ok := timeLayoutParams[ci.fn.Name]
				if !ok {
					return true
				}
				if arg := callArg(call, idx, "layout"); arg != nil {
					c.checkTimeLayout(arg)
				}
				return true
			})
		}
	}
	c.file = nil
}

// callArg is the argument of parameter idx (named name) of a call.
func callArg(call *ast.CallExpr, idx int, name string) ast.Expr {
	pos := 0
	for _, a := range call.Args {
		if a.Name != nil {
			if a.Name.Name == name {
				return a.Value
			}
			continue
		}
		if pos == idx {
			return a.Value
		}
		pos++
	}
	return nil
}

// checkTimeLayout reports a constant layout that is bad. The diagnostic
// is at the string literal that holds the layout (the argument, or the
// value of a constant declared in user code); a std constant is valid.
func (c *checker) checkTimeLayout(arg ast.Expr) {
	layout, ok := c.constStr(arg, 0)
	if !ok {
		return
	}
	sp := arg.Span()
	var lit *ast.StringLit
	switch x := unparen(arg).(type) {
	case *ast.StringLit:
		lit = x
	case *ast.Ident, *ast.SelectorExpr:
		co, _ := c.rec.ref[x].(*ConstObj)
		if co == nil || co.Module.IsStd {
			return
		}
		if l, ok := unparen(co.Decl.Value).(*ast.StringLit); ok {
			if co.File != nil && co.File != c.file {
				saved := c.file
				c.file = co.File
				defer func() { c.file = saved }()
			}
			lit, sp = l, l.Sp
		}
	}
	if c.timeLayoutHabit(layout, sp, lit) {
		return
	}
	_, msg, _ := timeLayoutParse(layout)
	if msg == "" {
		return
	}
	d := c.errorf(diag.TimeLayout, sp, "bad time layout %s: %s", xoQuote(layout), msg)
	if d == nil {
		return
	}
	note(d, token.Span{}, "layout fields are named and lowercase: {year}-{month}-{day} {hour}:{minute}:{second}; see std.md 13.3, or the constants time.RFC3339, time.RFC1123, time.DATE, time.TIME")
	// did you mean: every unknown field with one close name
	fixed, changed := layout, false
	for _, f := range layoutFields(layout) {
		if _, known := tlFieldArgs[f.name]; known {
			continue
		}
		sugg := nearest(f.name, tlFieldNames())
		if len(sugg) == 0 || !confidentGuess(f.name, sugg) {
			changed = false
			break
		}
		repl := "{" + sugg[0]
		if f.arg != "" {
			repl += ":" + f.arg
		}
		repl += "}"
		note(d, token.Span{}, "did you mean %s?", repl)
		fixed = strings.Replace(fixed, f.text, repl, 1)
		changed = true
	}
	if changed && lit != nil {
		if _, msg, _ := timeLayoutParse(fixed); msg == "" {
			fix(d, "write "+xoQuote(fixed), replaceEdit(lit.Sp, xoQuote(fixed)))
		}
	}
}

func tlFieldNames() []string {
	out := make([]string, 0, len(tlFieldArgs))
	for k := range tlFieldArgs {
		out = append(out, k)
	}
	return out
}

// layoutField is one `{name:arg}` of a layout, as written.
type layoutField struct{ text, name, arg string }

func layoutFields(layout string) []layoutField {
	var out []layoutField
	for i := 0; i < len(layout); i++ {
		if layout[i] != '{' {
			continue
		}
		j := strings.IndexAny(layout[i+1:], "{}")
		if j < 0 || layout[i+1+j] == '{' {
			continue
		}
		body := layout[i+1 : i+1+j]
		name, arg, _ := strings.Cut(body, ":")
		out = append(out, layoutField{text: "{" + body + "}", name: name, arg: arg})
		i += 1 + j
	}
	return out
}

// xoQuote writes s as an Xo string literal.
func xoQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\', '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ---------------------------------------------------------------------------
// Habits (XO0177)

// letterFields maps the date pattern letters of Java, C#, Swift, and
// moment.js to the named field ("" when there is no one rewrite).
var letterFields = map[string]string{
	"yyyy": "{year}", "YYYY": "{year}", "uuuu": "{year}", "yy": "", "YY": "",
	"MMMM": "{month:name}", "MMM": "{month:short}", "MM": "{month}", "M": "{month:1}",
	"dd": "{day}", "d": "{day:1}", "DD": "", "D": "",
	"dddd": "{weekday:name}", "ddd": "{weekday:short}", "EEEE": "{weekday:name}", "EEE": "{weekday:short}",
	"HH": "{hour}", "H": "", "hh": "{hour12}", "h": "",
	"mm": "{minute}", "m": "", "ss": "{second}", "s": "",
	"SSS": "{frac:3}", "SSSSSS": "{frac:6}", "SSSSSSSSS": "{frac:9}", "fff": "{frac:3}", "ffffff": "{frac:6}",
	"a": "{ampm}", "tt": "{ampm}", "A": "{ampm}",
	"XXX": "{offset}", "xxx": "", "Z": "{offset:compact}", "ZZ": "{offset:compact}", "z": "{zone}", "zzz": "",
}

// strongLetters are the letter runs that mark an unbraced layout as a
// letter pattern.
var strongLetters = map[string]bool{"yyyy": true, "YYYY": true, "MM": true, "dd": true, "HH": true, "mm": true, "ss": true}

// percentFields maps strftime codes to named fields ("" when there is no
// one rewrite).
var percentFields = map[byte]string{
	'Y': "{year}", 'm': "{month}", 'd': "{day}", 'H': "{hour}", 'M': "{minute}", 'S': "{second}",
	'b': "{month:short}", 'h': "{month:short}", 'B': "{month:name}", 'a': "{weekday:short}", 'A': "{weekday:name}",
	'p': "{ampm}", 'I': "{hour12}", 'z': "{offset:compact}", 'Z': "{zone}",
	'F': "{year}-{month}-{day}", 'T': "{hour}:{minute}:{second}", 'R': "{hour}:{minute}",
	'y': "", 'e': "", 'j': "", 'f': "", 'c': "", 'x': "", 'X': "", 'D': "", 'u': "", 'w': "", 's': "", 'n': "", 't': "",
}

// goRefTokens are the pieces of Go's reference date, longest first.
var goRefTokens = []struct{ tok, field string }{
	{"Z07:00", "{offset}"}, {".000000000", "{frac:9}"}, {".999999999", "{frac}"}, {".000000", "{frac:6}"}, {".000", "{frac:3}"},
	{"January", "{month:name}"}, {"Monday", "{weekday:name}"}, {"-07:00", ""}, {"-0700", "{offset:compact}"},
	{"2006", "{year}"}, {"Jan", "{month:short}"}, {"Mon", "{weekday:short}"}, {"MST", "{zone}"},
	{"01", "{month}"}, {"02", "{day}"}, {"15", "{hour}"}, {"03", "{hour12}"}, {"04", "{minute}"}, {"05", "{second}"},
	{"PM", "{ampm}"}, {"_2", ""}, {".999", ""}, {".999999", ""}, {"06", ""},
}

// timeLayoutHabit reports a layout written in another language's date
// pattern syntax. It reports whether it did.
func (c *checker) timeLayoutHabit(layout string, sp token.Span, lit *ast.StringLit) bool {
	var from string
	var b strings.Builder
	var seen []string
	fixable, found := true, false
	add := func(src, field string) {
		found = true
		if field == "" {
			fixable = false
		}
		if len(seen) < 4 && !tlContains(seen, src) {
			seen = append(seen, src)
		}
		b.WriteString(field)
	}
	switch {
	case strings.Contains(layout, "{"):
		// braced letter patterns: {yyyy}-{MM}-{dd}
		for i := 0; i < len(layout); i++ {
			if layout[i] == '{' {
				if j := strings.IndexAny(layout[i+1:], "{}"); j >= 0 && layout[i+1+j] == '}' {
					body := layout[i+1 : i+1+j]
					if _, known := tlFieldArgs[strings.SplitN(body, ":", 2)[0]]; !known {
						if f, ok := letterFields[body]; ok {
							add("{"+body+"}", f)
							i += 1 + j
							continue
						}
					}
					b.WriteString(layout[i : i+2+j])
					i += 1 + j
					continue
				}
			}
			b.WriteByte(layout[i])
		}
		from = "the date patterns of Java, C#, and Swift"
	case strings.Contains(layout, "%"):
		for i := 0; i < len(layout); i++ {
			if layout[i] == '%' && i+1 < len(layout) {
				nx := layout[i+1]
				if nx == '%' {
					b.WriteByte('%')
					i++
					continue
				}
				if f, ok := percentFields[nx]; ok {
					if nx == 'f' && strings.HasSuffix(b.String(), ".") {
						// `.%f` is a fraction of 6 digits
						s := b.String()
						b.Reset()
						b.WriteString(s[:len(s)-1])
						add("%f", "{frac:6}")
					} else {
						add(layout[i:i+2], f)
					}
					i++
					continue
				}
			}
			b.WriteByte(layout[i])
		}
		from = "the strftime codes of C, Python, Ruby, and PHP"
	case strings.Contains(layout, "2006") || strings.Contains(layout, "15:04"):
		for i := 0; i < len(layout); {
			matched := false
			for _, t := range goRefTokens {
				if strings.HasPrefix(layout[i:], t.tok) {
					add(t.tok, t.field)
					i += len(t.tok)
					matched = true
					break
				}
			}
			if !matched {
				b.WriteByte(layout[i])
				i++
			}
		}
		from = "Go's reference date"
	default:
		// unbraced letter patterns: yyyy-MM-dd HH:mm:ss
		strong := false
		for i := 0; i < len(layout); {
			ch := layout[i]
			if ch == '\'' {
				// a quoted literal: 'T'
				if j := strings.IndexByte(layout[i+1:], '\''); j >= 0 {
					b.WriteString(layout[i+1 : i+1+j])
					i += j + 2
					continue
				}
			}
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
				b.WriteByte(ch)
				i++
				continue
			}
			j := i
			for j < len(layout) && layout[j] == ch {
				j++
			}
			run := layout[i:j]
			if strongLetters[run] {
				strong = true
			}
			if f, ok := letterFields[run]; ok {
				if (run == "SSS" || run == "fff" || run == "SSSSSS" || run == "ffffff" || run == "SSSSSSSSS") && strings.HasSuffix(b.String(), ".") {
					s := b.String()
					b.Reset()
					b.WriteString(s[:len(s)-1])
				} else if strings.HasPrefix(f, "{frac") {
					f = ""
				}
				add(run, f)
			} else {
				add(run, "")
			}
			i = j
		}
		if !strong {
			return false
		}
		from = "the date patterns of Java, C#, and Swift"
	}
	if !found {
		return false
	}
	quoted := make([]string, len(seen))
	for i, s := range seen {
		quoted[i] = "`" + s + "`"
	}
	d := c.errorf(diag.ForeignSyntax, sp, "time layout written with %s (%s); Xo layouts use named, lowercase fields such as {year}-{month}-{day} (std.md 13.3)", from, strings.Join(quoted, ", "))
	if d == nil {
		return true
	}
	fixedLayout := b.String()
	if _, msg, _ := timeLayoutParse(fixedLayout); msg != "" {
		fixable = false
	}
	if fixable {
		note(d, token.Span{}, "write %s", xoQuote(fixedLayout))
		if lit != nil {
			fix(d, "write "+xoQuote(fixedLayout), replaceEdit(lit.Sp, xoQuote(fixedLayout)))
		}
	} else {
		note(d, token.Span{}, "fields: {year} {month} {month:1} {month:short} {month:name} {day} {day:1} {weekday:short} {weekday:name} {hour} {hour12} {ampm} {minute} {second} {frac} {frac:3} {offset} {offset:compact} {zone}; a two digit year is not offered")
	}
	return true
}

func tlContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The layout parser of the runtime (std/rt/timefmt.go TimeLayoutParse),
// reduced to validation. Keep the two equal (TestTimeLayoutAgrees).

var tlFieldArgs = map[string][]string{
	"year": {""}, "month": {"", "1", "short", "name"}, "day": {"", "1"},
	"weekday": {"short", "name"}, "hour": {""}, "hour12": {""}, "ampm": {""},
	"minute": {""}, "second": {""}, "frac": {"", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
	"offset": {"", "compact"}, "zone": {""}, "lbrace": {""}, "rbrace": {""},
}

// timeLayoutParse returns the error message of a bad layout and its byte
// offset, or "".
func timeLayoutParse(layout string) (int, string, int) {
	fields := 0
	hour12, ampm := -1, false
	for i := 0; i < len(layout); i++ {
		switch layout[i] {
		case '}':
			return 0, "unmatched } (a literal brace is written {rbrace})", i
		case '{':
		default:
			continue
		}
		j := strings.IndexAny(layout[i+1:], "{}")
		if j < 0 || layout[i+1+j] == '{' {
			return 0, "unclosed { (a literal brace is written {lbrace})", i
		}
		body := layout[i+1 : i+1+j]
		name, arg, hasArg := strings.Cut(body, ":")
		args, ok := tlFieldArgs[name]
		if !ok {
			return 0, "unknown field {" + body + "}", i
		}
		if hasArg && arg == "" || !hasArg && !tlContains(args, "") || hasArg && !tlContains(args, arg) {
			return 0, "{" + body + "}: " + tlArgsText(name, args), i
		}
		if name == "hour12" && hour12 < 0 {
			hour12 = i
		}
		if name == "ampm" {
			ampm = true
		}
		fields++
		i += 1 + j
	}
	if hour12 >= 0 && !ampm {
		return 0, "{hour12} needs {ampm} (or use {hour}, 00 to 23)", hour12
	}
	return fields, "", 0
}

func tlArgsText(name string, args []string) string {
	var forms []string
	for _, a := range args {
		if a == "" {
			forms = append(forms, "{"+name+"}")
		} else {
			forms = append(forms, "{"+name+":"+a+"}")
		}
	}
	if name == "frac" {
		forms = []string{"{frac}", "{frac:1} to {frac:9}"}
	}
	return "the forms are " + strings.Join(forms, ", ")
}

// TimeLayoutError is the error message and offset of a bad layout, or ""
// (for tests that compare with the runtime).
func TimeLayoutError(layout string) (string, int) {
	_, msg, at := timeLayoutParse(layout)
	return msg, at
}
