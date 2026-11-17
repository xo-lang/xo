package rt

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// std/toml (std.md 19, decision 0164): a TOML 1.0 parser whose document
// is decoded with the JSON rules (std.md 5). Native builds report it as
// XO1201 (planned).

// M_display is `toml: line 3: msg`.
func (e Toml_TomlErr) M_display(*Task) string {
	if e.F_line == 0 {
		return "toml: " + e.F_msg
	}
	return "toml: line " + strconv.FormatInt(e.F_line, 10) + ": " + e.F_msg
}

// M_display of a decode error is the text of the error it holds.
func (e Toml_DecodeErr) M_display(tk *Task) string {
	switch p := e.P.(type) {
	case Toml_DecodeErr_Syntax:
		return p.F_err.M_display(tk)
	case Toml_DecodeErr_Value:
		return p.F_err.M_display(tk)
	case Toml_DecodeErr_Strict:
		return "strict mode: fields in the document are missing in the target struct"
	}
	return ""
}

// M_display is `toml: line 3: unknown field a.b`.
func (e Toml_StrictErr) M_display(*Task) string {
	return "toml: line " + strconv.FormatInt(e.F_line, 10) + ": " + e.F_msg + " " + strings.Join(List_Slice(e.F_key), ".")
}

type tomlTable struct {
	keys     []string
	vals     map[string]any
	header   bool // defined by a [table] header
	dotted   bool // created by a dotted key
	inline   bool // an inline table: closed
	implicit bool // created on the way to a header or a dotted key
	// a strict decode's key spans (the whole dotted key of a key and
	// value), and a header table's key span in its header
	spans  map[string][2]int
	hs, he int
}

type tomlArray struct {
	elems  []any
	tables bool // an array of tables ([[x]])
}

type tomlParser struct {
	s      string
	i      int
	line   int
	err    *Toml_TomlErr
	ks, ke int // the span of the last key read
}

func newTomlTable() *tomlTable { return &tomlTable{vals: map[string]any{}, spans: map[string][2]int{}} }

func (t *tomlTable) set(k string, v any) {
	if _, ok := t.vals[k]; !ok {
		t.keys = append(t.keys, k)
	}
	t.vals[k] = v
}

// setAt is set with the key's span (the parser's last key).
func (p *tomlParser) setAt(t *tomlTable, k string, v any) {
	t.set(k, v)
	t.spans[k] = [2]int{p.ks, p.ke}
}

func (p *tomlParser) fail(msg string) {
	if p.err == nil {
		p.err = &Toml_TomlErr{F_line: int64(p.line), F_msg: msg}
	}
	panic(p)
}

func (p *tomlParser) eof() bool { return p.i >= len(p.s) }

func (p *tomlParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.s[p.i]
}

// ws skips spaces and tabs.
func (p *tomlParser) ws() {
	for !p.eof() && (p.s[p.i] == ' ' || p.s[p.i] == '\t') {
		p.i++
	}
}

// comment skips a comment up to (not over) the line end.
func (p *tomlParser) comment() {
	if p.peek() != '#' {
		return
	}
	for !p.eof() && p.s[p.i] != '\n' {
		c := p.s[p.i]
		if c < 0x20 && c != '\t' && !(c == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n') || c == 0x7f {
			p.fail("control character in a comment")
		}
		p.i++
	}
}

// eol expects the end of a line (after spaces and a comment).
func (p *tomlParser) eol() {
	p.ws()
	p.comment()
	switch {
	case p.eof():
	case p.s[p.i] == '\n':
		p.i++
		p.line++
	case p.s[p.i] == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n':
		p.i += 2
		p.line++
	default:
		p.fail("expected the end of the line, found " + strconv.QuoteRune(p.rune()))
	}
}

func (p *tomlParser) rune() rune {
	r, _ := utf8.DecodeRuneInString(p.s[p.i:])
	return r
}

// blank skips spaces, comments, and line breaks (inside arrays).
func (p *tomlParser) blank() {
	for {
		p.ws()
		p.comment()
		switch {
		case p.peek() == '\n':
			p.i++
			p.line++
		case p.peek() == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n':
			p.i += 2
			p.line++
		default:
			return
		}
	}
}

func bareKeyByte(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// key reads a dotted key.
func (p *tomlParser) key() []string {
	var parts []string
	for {
		p.ws()
		if len(parts) == 0 {
			p.ks = p.i
		}
		switch c := p.peek(); {
		case c == '"':
			if strings.HasPrefix(p.s[p.i:], `"""`) {
				p.fail("a key cannot be a multi-line string")
			}
			parts = append(parts, p.basicString())
		case c == '\'':
			if strings.HasPrefix(p.s[p.i:], "'''") {
				p.fail("a key cannot be a multi-line string")
			}
			parts = append(parts, p.literalString())
		case bareKeyByte(c):
			st := p.i
			for !p.eof() && bareKeyByte(p.s[p.i]) {
				p.i++
			}
			parts = append(parts, p.s[st:p.i])
		default:
			if p.eof() || c == '\n' || c == '\r' {
				p.fail("expected a key")
			}
			p.fail("expected a key, found " + strconv.QuoteRune(p.rune()))
		}
		p.ke = p.i
		p.ws()
		if p.peek() != '.' {
			return parts
		}
		p.i++
	}
}

func (p *tomlParser) basicString() string {
	p.i++ // "
	var b strings.Builder
	for {
		if p.eof() || p.s[p.i] == '\n' {
			p.fail("string not closed")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String()
		case c == '\\':
			p.escape(&b)
		case c < 0x20 && c != '\t' || c == 0x7f:
			p.fail("control character in a string")
		default:
			b.WriteByte(c)
			p.i++
		}
	}
}

func (p *tomlParser) escape(b *strings.Builder) {
	p.i++
	if p.eof() {
		p.fail("string not closed")
	}
	c := p.s[p.i]
	p.i++
	switch c {
	case 'b':
		b.WriteByte('\b')
	case 't':
		b.WriteByte('\t')
	case 'n':
		b.WriteByte('\n')
	case 'f':
		b.WriteByte('\f')
	case 'r':
		b.WriteByte('\r')
	case '"':
		b.WriteByte('"')
	case '\\':
		b.WriteByte('\\')
	case 'u', 'U':
		n := 4
		if c == 'U' {
			n = 8
		}
		if p.i+n > len(p.s) {
			p.fail("bad unicode escape")
		}
		v, err := strconv.ParseUint(p.s[p.i:p.i+n], 16, 32)
		if err != nil || !utf8.ValidRune(rune(v)) {
			p.fail("bad unicode escape \\" + string(c) + p.s[p.i:p.i+n])
		}
		b.WriteRune(rune(v))
		p.i += n
	default:
		r, _ := utf8.DecodeRuneInString(p.s[p.i-1:])
		p.fail("unknown escape \\" + string(r))
	}
}

func (p *tomlParser) literalString() string {
	p.i++ // '
	st := p.i
	for {
		if p.eof() || p.s[p.i] == '\n' {
			p.fail("string not closed")
		}
		c := p.s[p.i]
		if c == '\'' {
			s := p.s[st:p.i]
			p.i++
			return s
		}
		if c < 0x20 && c != '\t' || c == 0x7f {
			p.fail("control character in a string")
		}
		p.i++
	}
}

// multiline reads a """ or ”' string.
func (p *tomlParser) multiline(lit bool) string {
	q := `"""`
	if lit {
		q = "'''"
	}
	p.i += 3
	// a line break right after the opening quotes is trimmed
	if strings.HasPrefix(p.s[p.i:], "\r\n") {
		p.i += 2
		p.line++
	} else if p.peek() == '\n' {
		p.i++
		p.line++
	}
	var b strings.Builder
	for {
		if p.eof() {
			p.fail("multi-line string not closed")
		}
		if strings.HasPrefix(p.s[p.i:], q) {
			// up to two more quotes belong to the string
			n := 3
			for n < 5 && p.i+n < len(p.s) && p.s[p.i+n] == q[0] {
				n++
			}
			b.WriteString(p.s[p.i : p.i+n-3])
			p.i += n
			return b.String()
		}
		c := p.s[p.i]
		switch {
		case c == '\n':
			b.WriteByte('\n')
			p.i++
			p.line++
		case c == '\r' && p.i+1 < len(p.s) && p.s[p.i+1] == '\n':
			b.WriteString("\r\n")
			p.i += 2
			p.line++
		case !lit && c == '\\':
			// a line ending backslash trims the whitespace that follows
			j := p.i + 1
			for j < len(p.s) && (p.s[j] == ' ' || p.s[j] == '\t') {
				j++
			}
			if j < len(p.s) && (p.s[j] == '\n' || p.s[j] == '\r') {
				p.i = j
				for !p.eof() && (p.s[p.i] == ' ' || p.s[p.i] == '\t' || p.s[p.i] == '\n' || p.s[p.i] == '\r') {
					if p.s[p.i] == '\n' {
						p.line++
					}
					p.i++
				}
				continue
			}
			p.escape(&b)
		case c < 0x20 && c != '\t' || c == 0x7f:
			p.fail("control character in a string")
		default:
			b.WriteByte(c)
			p.i++
		}
	}
}

// value reads a value.
func (p *tomlParser) value() any {
	p.ws()
	if p.eof() {
		p.fail("expected a value")
	}
	rest := p.s[p.i:]
	switch c := p.s[p.i]; {
	case strings.HasPrefix(rest, `"""`):
		return p.multiline(false)
	case strings.HasPrefix(rest, "'''"):
		return p.multiline(true)
	case c == '"':
		return p.basicString()
	case c == '\'':
		return p.literalString()
	case c == '[':
		return p.array()
	case c == '{':
		return p.inlineTable()
	case strings.HasPrefix(rest, "true") && !tomlWordByte(rest, 4):
		p.i += 4
		return true
	case strings.HasPrefix(rest, "false") && !tomlWordByte(rest, 5):
		p.i += 5
		return false
	}
	return p.scalar()
}

func tomlWordByte(s string, i int) bool { return i < len(s) && bareKeyByte(s[i]) }

// scalar reads a number or a date and time.
func (p *tomlParser) scalar() any {
	st := p.i
	for !p.eof() {
		c := p.s[p.i]
		// a date and time may hold one space between date and time
		if c == ' ' && p.i-st == 10 && p.i+1 < len(p.s) && p.s[p.i+1] >= '0' && p.s[p.i+1] <= '9' && strings.Count(p.s[st:p.i], "-") == 2 {
			p.i++
			continue
		}
		if c == ' ' || c == '\t' || c == ',' || c == ']' || c == '}' || c == '#' || c == '\n' || c == '\r' {
			break
		}
		p.i++
	}
	tok := p.s[st:p.i]
	if tok == "" {
		p.fail("expected a value, found " + strconv.QuoteRune(p.rune()))
	}
	if v, ok := tomlDateTime(tok); ok {
		return v
	}
	if v, ok := tomlInt(tok); ok {
		return v
	}
	if v, ok := tomlFloat(tok); ok {
		if _, bad := v.(tomlNonFinite); bad {
			p.fail(tok + " cannot be decoded: JSON has no form for inf and nan")
		}
		return v
	}
	p.fail("bad value " + strconv.Quote(tok))
	return nil
}

// tomlUnderscores removes `_` between digits; false when one is misplaced.
func tomlUnderscores(s string, digit func(byte) bool) (string, bool) {
	if !strings.Contains(s, "_") {
		return s, true
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '_' && (i == 0 || i == len(s)-1 || !digit(s[i-1]) || !digit(s[i+1])) {
			return "", false
		}
	}
	return strings.ReplaceAll(s, "_", ""), true
}

func isDec(c byte) bool { return c >= '0' && c <= '9' }

func tomlInt(tok string) (any, bool) {
	base := 10
	digits := tok
	sign := ""
	switch {
	case strings.HasPrefix(tok, "0x"):
		base, digits = 16, tok[2:]
	case strings.HasPrefix(tok, "0o"):
		base, digits = 8, tok[2:]
	case strings.HasPrefix(tok, "0b"):
		base, digits = 2, tok[2:]
	default:
		if strings.HasPrefix(tok, "+") || strings.HasPrefix(tok, "-") {
			sign, digits = tok[:1], tok[1:]
		}
		if len(digits) > 1 && digits[0] == '0' {
			return nil, false // leading zeros
		}
	}
	isDigit := func(c byte) bool {
		switch base {
		case 16:
			return isDec(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		case 8:
			return c >= '0' && c <= '7'
		case 2:
			return c == '0' || c == '1'
		}
		return isDec(c)
	}
	d, ok := tomlUnderscores(digits, isDigit)
	if !ok || d == "" {
		return nil, false
	}
	for i := 0; i < len(d); i++ {
		if !isDigit(d[i]) {
			return nil, false
		}
	}
	n, err := strconv.ParseInt(sign+d, base, 64)
	if err != nil {
		return nil, false
	}
	return json.Number(strconv.FormatInt(n, 10)), true
}

func tomlFloat(tok string) (any, bool) {
	switch strings.TrimLeft(tok, "+-") {
	case "inf", "nan":
		return tomlNonFinite(tok), true
	}
	if strings.HasPrefix(tok, "inf") || strings.HasPrefix(tok, "nan") {
		return nil, false
	}
	d, ok := tomlUnderscores(tok, isDec)
	if !ok {
		return nil, false
	}
	// TOML: an integer part without leading zeros, then a fraction with
	// digits and/or an exponent
	s := d
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	intEnd := 0
	for intEnd < len(s) && isDec(s[intEnd]) {
		intEnd++
	}
	if intEnd == 0 || intEnd > 1 && s[0] == '0' || intEnd == len(s) {
		return nil, false
	}
	rest := s[intEnd:]
	if rest[0] == '.' {
		j := 1
		for j < len(rest) && isDec(rest[j]) {
			j++
		}
		if j == 1 {
			return nil, false
		}
		rest = rest[j:]
	}
	if rest != "" {
		if rest[0] != 'e' && rest[0] != 'E' {
			return nil, false
		}
		e := strings.TrimLeft(rest[1:], "+-")
		if len(rest[1:])-len(e) > 1 || e == "" {
			return nil, false
		}
		for i := 0; i < len(e); i++ {
			if !isDec(e[i]) {
				return nil, false
			}
		}
	}
	if _, err := strconv.ParseFloat(d, 64); err != nil {
		return nil, false // out of range
	}
	// the token itself is a JSON number once its `+` and underscores are
	// gone (TOML has digits on both sides of a point and no leading zeros)
	return json.Number(strings.TrimPrefix(d, "+")), true
}

// tomlNonFinite marks inf and nan, which JSON cannot hold (the parser
// reports them).
type tomlNonFinite string

// tomlDateTime recognizes the four TOML date and time forms and returns
// their RFC 3339 text (`T` and `Z` upper case, a space read as `T`).
func tomlDateTime(tok string) (any, bool) {
	num := func(s string) int { return int(s[0]-'0')*10 + int(s[1]-'0') }
	// RFC 3339's ranges, as time.Parse checks them: a day of its month,
	// hour 0 to 23, minute and second 0 to 59, an offset's hour 0 to 23.
	isDate := func(s string) bool {
		if !(len(s) == 10 && isDec(s[0]) && isDec(s[1]) && isDec(s[2]) && isDec(s[3]) && s[4] == '-' && isDec(s[5]) && isDec(s[6]) && s[7] == '-' && isDec(s[8]) && isDec(s[9])) {
			return false
		}
		y, m, d := num(s)*100+num(s[2:]), num(s[5:]), num(s[8:])
		days := 31
		switch m {
		case 4, 6, 9, 11:
			days = 30
		case 2:
			days = 28
			if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
				days = 29
			}
		}
		return m >= 1 && m <= 12 && d >= 1 && d <= days
	}
	isTime := func(s string) bool {
		if len(s) < 8 || !(isDec(s[0]) && isDec(s[1]) && s[2] == ':' && isDec(s[3]) && isDec(s[4]) && s[5] == ':' && isDec(s[6]) && isDec(s[7])) {
			return false
		}
		if num(s) > 23 || num(s[3:]) > 59 || num(s[6:]) > 59 {
			return false
		}
		rest := s[8:]
		if strings.HasPrefix(rest, ".") {
			j := 1
			for j < len(rest) && isDec(rest[j]) {
				j++
			}
			if j == 1 {
				return false
			}
			rest = rest[j:]
		}
		switch {
		case rest == "", rest == "Z", rest == "z":
			return true
		case len(rest) == 6 && (rest[0] == '+' || rest[0] == '-') && isDec(rest[1]) && isDec(rest[2]) && rest[3] == ':' && isDec(rest[4]) && isDec(rest[5]):
			return num(rest[1:]) <= 23 && num(rest[4:]) <= 59
		}
		return false
	}
	switch {
	case isDate(tok):
		return tok, true
	case len(tok) > 11 && isDate(tok[:10]) && (tok[10] == 'T' || tok[10] == 't' || tok[10] == ' ') && isTime(tok[11:]):
		t := tok[11:]
		if strings.HasSuffix(t, "z") {
			t = strings.TrimSuffix(t, "z") + "Z"
		}
		return tok[:10] + "T" + t, true
	case isTime(tok) && !strings.ContainsAny(tok[8:], "Zz+-"):
		return tok, true
	}
	return nil, false
}

func (p *tomlParser) array() any {
	p.i++ // [
	arr := &tomlArray{}
	for {
		p.blank()
		if p.peek() == ']' {
			p.i++
			return arr
		}
		arr.elems = append(arr.elems, p.value())
		p.blank()
		switch p.peek() {
		case ',':
			p.i++
		case ']':
			p.i++
			return arr
		default:
			if p.eof() {
				p.fail("array not closed")
			}
			p.fail("expected , or ] in an array, found " + strconv.QuoteRune(p.rune()))
		}
	}
}

func (p *tomlParser) inlineTable() any {
	p.i++ // {
	t := newTomlTable()
	p.ws()
	if p.peek() == '}' {
		p.i++
		t.inline = true
		return t
	}
	for {
		k := p.key()
		ks, ke := p.ks, p.ke
		p.ws()
		if p.peek() != '=' {
			p.fail("expected = after a key")
		}
		p.i++
		v := p.value()
		p.ks, p.ke = ks, ke
		p.assign(t, k, v)
		p.ws()
		switch p.peek() {
		case ',':
			p.i++
			p.ws()
			if p.peek() == '}' {
				p.fail("trailing comma in an inline table")
			}
		case '}':
			p.i++
			t.inline = true
			return t
		default:
			p.fail("expected , or } in an inline table")
		}
	}
}

// assign sets the dotted key k of t to v.
func (p *tomlParser) assign(t *tomlTable, k []string, v any) {
	for _, part := range k[:len(k)-1] {
		x, ok := t.vals[part]
		if !ok {
			n := newTomlTable()
			n.dotted = true
			p.setAt(t, part, n)
			t = n
			continue
		}
		sub, ok := x.(*tomlTable)
		if !ok || sub.inline || sub.header && !sub.dotted {
			p.fail("key " + strconv.Quote(strings.Join(k, ".")) + " is already defined")
		}
		t = sub
	}
	last := k[len(k)-1]
	if _, ok := t.vals[last]; ok {
		p.fail("key " + strconv.Quote(strings.Join(k, ".")) + " is already defined")
	}
	p.setAt(t, last, v)
}

// header walks to (creating) the table of a [header] or [[header]].
func (p *tomlParser) header(root *tomlTable, k []string, array bool) *tomlTable {
	t := root
	for i, part := range k {
		last := i == len(k)-1
		x, ok := t.vals[part]
		if !ok {
			if last && array {
				n := newTomlTable()
				n.header = true
				n.hs, n.he = p.ks, p.ke
				p.setAt(t, part, &tomlArray{elems: []any{n}, tables: true})
				return n
			}
			n := newTomlTable()
			n.implicit = !last
			n.header = last
			n.hs, n.he = p.ks, p.ke
			p.setAt(t, part, n)
			t = n
			continue
		}
		switch v := x.(type) {
		case *tomlTable:
			if v.inline {
				p.fail("table " + strconv.Quote(strings.Join(k[:i+1], ".")) + " is an inline table")
			}
			if last {
				if array {
					p.fail("key " + strconv.Quote(strings.Join(k, ".")) + " is already a table")
				}
				if v.header || v.dotted && !v.implicit {
					p.fail("table " + strconv.Quote(strings.Join(k, ".")) + " is already defined")
				}
				v.header, v.implicit = true, false
				v.hs, v.he = p.ks, p.ke
			}
			t = v
		case *tomlArray:
			if !v.tables {
				p.fail("key " + strconv.Quote(strings.Join(k[:i+1], ".")) + " is already an array")
			}
			if last {
				if !array {
					p.fail("table " + strconv.Quote(strings.Join(k, ".")) + " is already an array of tables")
				}
				n := newTomlTable()
				n.header = true
				n.hs, n.he = p.ks, p.ke
				v.elems = append(v.elems, n)
				return n
			}
			t = v.elems[len(v.elems)-1].(*tomlTable)
		default:
			p.fail("key " + strconv.Quote(strings.Join(k[:i+1], ".")) + " is already a value")
		}
	}
	return t
}

// tomlParse parses a document.
func tomlParse(text string) (root *tomlTable, err *Toml_TomlErr) {
	p := &tomlParser{s: text, line: 1}
	root = newTomlTable()
	defer func() {
		if r := recover(); r != nil {
			if r != p {
				panic(r)
			}
			root, err = nil, p.err
		}
	}()
	if !utf8.ValidString(text) {
		p.fail("the document is not UTF-8")
	}
	cur := root
	for {
		p.blank()
		if p.eof() {
			return root, nil
		}
		if p.peek() == '[' {
			array := strings.HasPrefix(p.s[p.i:], "[[")
			if array {
				p.i += 2
			} else {
				p.i++
			}
			k := p.key()
			p.ws()
			if array {
				if !strings.HasPrefix(p.s[p.i:], "]]") {
					p.fail("expected ]] after an array of tables header")
				}
				p.i += 2
			} else {
				if p.peek() != ']' {
					p.fail("expected ] after a table header")
				}
				p.i++
			}
			cur = p.header(root, k, array)
			p.eol()
			continue
		}
		k := p.key()
		ks, ke := p.ks, p.ke
		p.ws()
		if p.peek() != '=' {
			p.fail("expected = after a key")
		}
		p.i++
		p.ws()
		if p.eof() || p.peek() == '\n' || p.peek() == '\r' || p.peek() == '#' {
			p.fail("expected a value after =")
		}
		v := p.value()
		p.ks, p.ke = ks, ke
		p.assign(cur, k, v)
		p.eol()
	}
}

// tomlTree converts a parsed document into the parsed JSON form of
// std/rt's decoder; inf and nan are errors.
func tomlTree(v any) (any, string) {
	switch x := v.(type) {
	case *tomlTable:
		obj := &jsonObj{vals: map[string]any{}}
		for _, k := range x.keys {
			e, msg := tomlTree(x.vals[k])
			if msg != "" {
				return nil, msg
			}
			obj.keys = append(obj.keys, k)
			obj.vals[k] = e
		}
		return obj, ""
	case *tomlArray:
		arr := make([]any, len(x.elems))
		for i, e := range x.elems {
			t, msg := tomlTree(e)
			if msg != "" {
				return nil, msg
			}
			arr[i] = t
		}
		return arr, ""
	}
	return v, ""
}

// tomlJSON writes the JSON text of a converted tree.
func tomlJSON(b *strings.Builder, v any) {
	switch x := v.(type) {
	case *jsonObj:
		b.WriteByte('{')
		for i, k := range x.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			tomlJSONString(b, k)
			b.WriteByte(':')
			tomlJSON(b, x.vals[k])
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			tomlJSON(b, e)
		}
		b.WriteByte(']')
	case json.Number:
		b.WriteString(string(x))
	case string:
		tomlJSONString(b, x)
	case bool:
		b.WriteString(strconv.FormatBool(x))
	}
}

const tomlHex = "0123456789abcdef"

func tomlJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			b.WriteString(`\u00`)
			b.WriteByte(tomlHex[c>>4])
			b.WriteByte(tomlHex[c&15])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}

func tomlDoc(text string) (any, *Toml_TomlErr) {
	root, err := tomlParse(text)
	if err != nil {
		return nil, err
	}
	tree, msg := tomlTree(root)
	if msg != "" {
		return nil, &Toml_TomlErr{F_line: 0, F_msg: msg}
	}
	return tree, nil
}

// Toml_to_json is `toml.to_json(text)`.
func Toml_to_json(_ *Task, text string) Result[string, Toml_TomlErr] {
	tree, err := tomlDoc(text)
	if err != nil {
		return Err[string](*err)
	}
	var b strings.Builder
	tomlJSON(&b, tree)
	return Ok[string, Toml_TomlErr](b.String())
}

// Toml_decode is `toml.decode[T](text)`.
func Toml_decode[T any](tk *Task, text string, strict bool) Result[T, Toml_DecodeErr] {
	root, err := tomlParse(text)
	if err != nil {
		return Err[T](Toml_DecodeErr{Tag: 1, P: Toml_DecodeErr_Syntax{F_err: *err}})
	}
	tree, msg := tomlTree(root)
	if msg != "" {
		return Err[T](Toml_DecodeErr{Tag: 1, P: Toml_DecodeErr_Syntax{F_err: Toml_TomlErr{F_msg: msg}}})
	}
	var v T
	if e := JsonDec(tk, &v, tree, JsonRoot); e != nil {
		return Err[T](Toml_DecodeErr{Tag: 2, P: Toml_DecodeErr_Value{F_err: *e}})
	}
	if strict {
		if errs := tomlStrict[T](tk, text, root, tree, v); len(errs) > 0 {
			return Err[T](Toml_DecodeErr{Tag: 3, P: Toml_DecodeErr_Strict{F_errs: ListFromSlice(errs)}})
		}
	}
	return Ok[T, Toml_DecodeErr](v)
}

// Strict decode, as std/native/toml.xo's: a key the decoded value's JSON
// lacks is probed (its value replaced by a string) and is unknown when T
// decodes the same value.

const tomlStrictProbe = "toml strict probe 7f3c"

type tomlSpot struct {
	start, end int
	keys       []string
	msg        string
}

func tomlStrict[T any](tk *Task, text string, root *tomlTable, tree any, v T) []Toml_StrictErr {
	base := string(Json_encode(tk, v))
	got, _ := jsonParse([]byte(base))
	unknown := func(path []any) bool {
		var w T
		if e := JsonDec(tk, &w, tomlReplace(tree, path), JsonRoot); e != nil {
			return false
		}
		return string(Json_encode(tk, w)) == base
	}
	var spots []tomlSpot
	var walk func(x any, path []any, keys []string, got any)
	walk = func(x any, path []any, keys []string, got any) {
		switch t := x.(type) {
		case *tomlTable:
			o, ok := got.(*jsonObj)
			if !ok {
				return
			}
			for _, k := range t.keys {
				kp := append(append([]any{}, path...), k)
				kk := append(append([]string{}, keys...), k)
				g, ok := o.vals[k]
				if ok {
					walk(t.vals[k], kp, kk, g)
				} else if unknown(kp) {
					spots = append(spots, tomlMissing(t, k, kk, true)...)
				}
			}
		case *tomlArray:
			items, ok := got.([]any)
			if !ok || len(items) != len(t.elems) {
				return
			}
			for i, e := range t.elems {
				walk(e, append(append([]any{}, path...), i), keys, items[i])
			}
		}
	}
	walk(root, nil, nil, got)
	sort.SliceStable(spots, func(i, j int) bool { return spots[i].start < spots[j].start })
	var out []Toml_StrictErr
	for _, x := range spots {
		line, ls := 1, 0
		for j := 0; j < x.start; j++ {
			if text[j] == '\n' {
				line++
				ls = j + 1
			}
		}
		col := x.start - ls + 1
		out = append(out, Toml_StrictErr{F_line: int64(line), F_column: int64(col), F_key: ListFromSlice(x.keys), F_msg: x.msg, F_context: tomlContext(text, line-1, col, x.end-x.start, x.msg)})
	}
	return out
}

// tomlReplace is tree with the value at path (keys and indexes) the probe.
func tomlReplace(x any, path []any) any {
	if len(path) == 0 {
		return tomlStrictProbe
	}
	switch k := path[0].(type) {
	case string:
		o, ok := x.(*jsonObj)
		if !ok {
			return x
		}
		n := &jsonObj{keys: o.keys, vals: make(map[string]any, len(o.vals))}
		for kk, vv := range o.vals {
			n.vals[kk] = vv
		}
		n.vals[k] = tomlReplace(o.vals[k], path[1:])
		return n
	case int:
		a, ok := x.([]any)
		if !ok {
			return x
		}
		n := append([]any{}, a...)
		n[k] = tomlReplace(a[k], path[1:])
		return n
	}
	return x
}

// tomlMissing reports t's key k as go-toml does (std/native/toml.xo
// strict_missing).
func tomlMissing(t *tomlTable, k string, keys []string, kvs bool) []tomlSpot {
	sp := t.spans[k]
	field := tomlSpot{start: sp[0], end: sp[1], keys: keys, msg: "unknown field"}
	var out []tomlSpot
	switch x := t.vals[k].(type) {
	case *tomlTable:
		switch {
		case x.inline:
			if kvs {
				out = append(out, field)
			}
		case x.header:
			out = append(out, tomlSpot{start: x.hs, end: x.he, keys: keys, msg: "missing table"})
			out = append(out, tomlMissingAll(x, keys, false)...)
		default:
			out = append(out, tomlMissingAll(x, keys, kvs && x.dotted)...)
		}
	case *tomlArray:
		if !x.tables {
			if kvs {
				out = append(out, field)
			}
			break
		}
		for _, e := range x.elems {
			et := e.(*tomlTable)
			out = append(out, tomlSpot{start: et.hs, end: et.he, keys: keys, msg: "missing table"})
			out = append(out, tomlMissingAll(et, keys, false)...)
		}
	default:
		if kvs {
			out = append(out, field)
		}
	}
	return out
}

func tomlMissingAll(t *tomlTable, keys []string, kvs bool) []tomlSpot {
	var out []tomlSpot
	for _, k := range t.keys {
		out = append(out, tomlMissing(t, k, append(append([]string{}, keys...), k), kvs)...)
	}
	return out
}

// tomlContext is go-toml's buildHumanContext.
func tomlContext(doc string, li, column, n int, msg string) string {
	lines := strings.Split(doc, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	first, last := max(li-3, 0), min(li+3, len(lines)-1)
	for first < li && lines[first] == "" {
		first++
	}
	for last > li && lines[last] == "" {
		last--
	}
	width := len(strconv.Itoa(last + 1))
	var b strings.Builder
	line := func(idx int) {
		num := strconv.Itoa(idx + 1)
		b.WriteString(strings.Repeat(" ", width-len(num)))
		b.WriteString(num)
		b.WriteByte('|')
		if lines[idx] != "" {
			b.WriteByte(' ')
			b.WriteString(lines[idx])
		}
		b.WriteByte('\n')
	}
	for idx := first; idx <= li; idx++ {
		line(idx)
	}
	b.WriteString(strings.Repeat(" ", width))
	b.WriteString("| ")
	b.WriteString(strings.Repeat(" ", column-1))
	tildes := n
	if li < len(lines) {
		if avail := len(lines[li]) - column + 1; tildes > avail {
			tildes = avail
		}
	}
	b.WriteString(strings.Repeat("~", max(tildes, 1)))
	if msg != "" {
		b.WriteByte(' ')
		b.WriteString(msg)
	}
	b.WriteByte('\n')
	for idx := li + 1; idx <= last; idx++ {
		line(idx)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Toml_encode is `toml.encode(v)`: pelletier/go-toml/v2's Marshal layout
// over v's JSON tree, as std/native/toml.xo encode.
func Toml_encode[T any](tk *Task, v T) Result[string, Toml_TomlErr] {
	return Toml_encode_with(tk, v, Toml_EncodeOptions{F_indent: "  "})
}

// Toml_encode_with is `toml.encode_with(v, opts)`: go-toml's Encoder
// settings.
func Toml_encode_with[T any](tk *Task, v T, opts Toml_EncodeOptions) Result[string, Toml_TomlErr] {
	x, perr := jsonParse([]byte(Json_encode(tk, v)))
	if perr != nil {
		x = nil
	}
	o, ok := x.(*jsonObj)
	if !ok {
		return Err[string](Toml_TomlErr{F_msg: "cannot encode " + tomlKind(x) + " as a document root"})
	}
	e := &tomlEnc{inline: opts.F_tables_inline, multiline: opts.F_arrays_multiline, indentTables: opts.F_indent_tables, sym: opts.F_indent}
	if err := e.table(nil, o, 0); err != nil {
		return Err[string](*err)
	}
	return Ok[string, Toml_TomlErr](e.b.String())
}

type tomlEnc struct {
	b            strings.Builder
	lastHeader   bool
	inline       bool
	multiline    bool
	indentTables bool
	sym          string
}

// entryTable is go-toml's entryIsTable: a table or an array of tables,
// unless tables are inline.
func (e *tomlEnc) entryTable(x any) bool { return !e.inline && tomlIsTable(x) }

// indent is go-toml's writeIndent.
func (e *tomlEnc) indent(n int) {
	if e.indentTables {
		for i := 0; i < n; i++ {
			e.b.WriteString(e.sym)
		}
	}
}

func tomlKind(x any) string {
	switch t := x.(type) {
	case nil:
		return "a None"
	case bool:
		return "a Bool"
	case json.Number:
		if _, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return "an Int"
		}
		return "a Float"
	case string:
		return "a Str"
	case []any:
		return "a List"
	}
	return "a record"
}

func tomlTableArray(x any) bool {
	l, ok := x.([]any)
	if !ok || len(l) == 0 {
		return false
	}
	for _, e := range l {
		if _, ok := e.(*jsonObj); !ok {
			return false
		}
	}
	return true
}

func tomlIsTable(x any) bool {
	if _, ok := x.(*jsonObj); ok {
		return true
	}
	return tomlTableArray(x)
}

func (e *tomlEnc) table(keys []string, o *jsonObj, depth int) *Toml_TomlErr {
	for _, k := range o.keys {
		v := o.vals[k]
		if v == nil || e.entryTable(v) {
			continue
		}
		e.indent(depth)
		tomlKey(&e.b, k)
		e.b.WriteString(" = ")
		vd := 0
		if e.indentTables {
			vd = depth
		}
		if err := e.value(v, vd); err != nil {
			return err
		}
		e.b.WriteByte('\n')
		e.lastHeader = false
	}
	for _, k := range o.keys {
		v := o.vals[k]
		if v == nil || !e.entryTable(v) {
			continue
		}
		sub := append(keys[:len(keys):len(keys)], k)
		if t, ok := v.(*jsonObj); ok {
			e.header(sub, false, depth)
			if err := e.table(sub, t, depth+1); err != nil {
				return err
			}
			continue
		}
		for _, x := range v.([]any) {
			e.header(sub, true, depth)
			if err := e.table(sub, x.(*jsonObj), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *tomlEnc) header(keys []string, array bool, depth int) {
	if e.b.Len() > 0 && !e.lastHeader {
		e.b.WriteByte('\n')
	}
	e.indent(depth)
	e.b.WriteByte('[')
	if array {
		e.b.WriteByte('[')
	}
	for i, k := range keys {
		if i > 0 {
			e.b.WriteByte('.')
		}
		tomlKey(&e.b, k)
	}
	e.b.WriteByte(']')
	if array {
		e.b.WriteByte(']')
	}
	e.b.WriteByte('\n')
	e.lastHeader = true
}

func tomlKey(b *strings.Builder, k string) {
	bare := k != ""
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			bare = false
			break
		}
	}
	if bare {
		b.WriteString(k)
		return
	}
	tomlString(b, k)
}

func (e *tomlEnc) value(x any, depth int) *Toml_TomlErr {
	b := &e.b
	switch t := x.(type) {
	case nil:
		return &Toml_TomlErr{F_msg: "cannot encode a None in an array"}
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			b.WriteString(strconv.FormatInt(n, 10))
			return nil
		}
		f, _ := strconv.ParseFloat(string(t), 64)
		s := strconv.FormatFloat(f, 'f', -1, 64)
		b.WriteString(s)
		if !strings.ContainsAny(s, ".eE") {
			b.WriteString(".0")
		}
	case string:
		tomlString(b, t)
	case []any:
		b.WriteByte('[')
		if e.multiline && len(t) > 0 {
			// go-toml's appendArray multiline: an element a line, one
			// indent deeper
			for i, x := range t {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteByte('\n')
				for j := 0; j <= depth; j++ {
					b.WriteString(e.sym)
				}
				if err := e.value(x, depth+1); err != nil {
					return err
				}
			}
			b.WriteByte('\n')
			for j := 0; j < depth; j++ {
				b.WriteString(e.sym)
			}
		} else {
			for i, x := range t {
				if i > 0 {
					b.WriteString(", ")
				}
				if err := e.value(x, depth); err != nil {
					return err
				}
			}
		}
		b.WriteByte(']')
	case *jsonObj:
		b.WriteByte('{')
		first := true
		for _, k := range t.keys {
			v := t.vals[k]
			if v == nil {
				continue
			}
			if !first {
				b.WriteString(", ")
			}
			first = false
			tomlKey(b, k)
			b.WriteString(" = ")
			if err := e.value(v, depth); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	}
	return nil
}

// tomlString is go-toml's appendString: a literal string when it can be,
// else a basic string with Go's escapes.
func tomlString(b *strings.Builder, s string) {
	lit := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '\'' || c == 0x7f || c < 0x20 {
			lit = false
			break
		}
	}
	if lit && utf8.ValidString(s) {
		b.WriteByte('\'')
		b.WriteString(s)
		b.WriteByte('\'')
		return
	}
	const hex = "0123456789ABCDEF"
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			b.WriteString(`\u00`)
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}
