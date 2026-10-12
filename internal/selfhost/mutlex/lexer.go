// Package mutlex is a frozen copy of internal/lexer and internal/token
// (commit a6838e4, 2026-10-12) for the mutation generators of
// internal/selfhost (token level edits of .xo sources), so that those
// tests survive the deletion of the Go implementation (decision 0178,
// stage 7). Diagnostics are counted, not kept (diag.go).
//
// Do not change it to follow the language: the mutated corpora depend on
// these token boundaries, and the recorded Go outputs
// (internal/selfhost/testdata/gogoldens) are keyed by the corpora's
// content. A change here changes the corpora; record the hashes again
// (scripts/selfhost-record-goldens.sh) while the Go implementation
// exists, or regenerate them from the Xo implementation after.
//
// The original package comment follows.
//
// Newlines become Newline when the last token on the line can end a
// statement (core 1.3), except when the next non blank, non comment line
// starts with a single `.`, which continues the previous line. String
// literals with `${expr}` interpolation are returned as one token holding the
// literal parts and the raw source of each expression; the parser lexes and
// parses those expressions with LexRange.
package mutlex

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Comment is a `//` or `///` comment. Doc is set for `///`.
type Comment struct {
	Span Span
	Text string // without the leading slashes and one following space
	Doc  bool
}

// Result is the output of Lex.
type Result struct {
	Tokens   []Token
	Comments []Comment
}

type lexer struct {
	file  string
	src   []byte
	off   int
	end   int
	line  int
	col   int
	diags *List

	// rangeMode is set when lexing an interpolated expression: no newline
	// tokens are produced.
	rangeMode bool

	toks     []Token
	comments []Comment
	last     Kind // last significant token, for newline insertion
}

// Lex tokenizes a whole file. Errors are added to diags. The token slice
// always ends with EOF.
func Lex(file string, src []byte, diags *List) Result {
	l := &lexer{file: file, src: src, end: len(src), line: 1, col: 1, diags: diags, last: Newline}
	l.run()
	return Result{Tokens: l.toks, Comments: l.comments}
}

// LexRange tokenizes src[start.Offset:end] with positions continuing from
// start. It is used for interpolated expressions and never emits newline
// tokens.
func LexRange(file string, src []byte, start Pos, end int, diags *List) []Token {
	l := &lexer{file: file, src: src, off: start.Offset, end: end, line: start.Line, col: start.Col, diags: diags, rangeMode: true, last: Newline}
	l.run()
	return l.toks
}

func (l *lexer) pos() Pos { return Pos{Offset: l.off, Line: l.line, Col: l.col} }

func (l *lexer) span(start Pos) Span {
	return Span{File: l.file, Start: start, End: l.pos()}
}

func (l *lexer) peek(n int) byte {
	if l.off+n < l.end {
		return l.src[l.off+n]
	}
	return 0
}

// advance moves past one byte, tracking lines and columns.
func (l *lexer) advance() {
	if l.off >= l.end {
		return
	}
	if l.src[l.off] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.off++
}

func (l *lexer) advanceN(n int) {
	for i := 0; i < n; i++ {
		l.advance()
	}
}

func (l *lexer) errorf(code Code, sp Span, format string, args ...any) {
	l.diags.Errorf(code, sp, format, args...)
}

func (l *lexer) emit(kind Kind, start Pos) *Token {
	sp := l.span(start)
	l.toks = append(l.toks, Token{Kind: kind, Span: sp, Text: string(l.src[start.Offset:l.off])})
	l.last = kind
	return &l.toks[len(l.toks)-1]
}

// endsStatement reports whether a newline after k terminates a statement
// (core 1.3). `???` counts as a literal.
func endsStatement(k Kind) bool {
	switch k {
	case Ident, Int, Float, Duration, Rune, String,
		RawString, Bytes, ImportPath, Hole,
		RParen, RBrack, RBrace, Question,
		Return, Break, Continue:
		return true
	}
	return false
}

func (l *lexer) run() {
	for {
		l.skipSpaceAndComments()
		if l.off >= l.end {
			if !l.rangeMode && endsStatement(l.last) {
				p := l.pos()
				l.toks = append(l.toks, Token{Kind: Newline, Span: Span{File: l.file, Start: p, End: p}})
			}
			p := l.pos()
			l.toks = append(l.toks, Token{Kind: EOF, Span: Span{File: l.file, Start: p, End: p}})
			return
		}
		l.next()
	}
}

// skipSpaceAndComments skips blanks and comments and handles newlines.
func (l *lexer) skipSpaceAndComments() {
	for l.off < l.end {
		c := l.src[l.off]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.advance()
		case c == '\n':
			start := l.pos()
			l.advance()
			if !l.rangeMode && endsStatement(l.last) && !l.continuesLine() {
				l.toks = append(l.toks, Token{Kind: Newline, Span: l.span(start), Text: "\n"})
				l.last = Newline
			}
		case c == '/' && l.peek(1) == '/':
			l.comment()
		default:
			return
		}
	}
}

// continuesLine reports whether the next line holding a token starts with a
// single `.` (core 1.3). Blank lines and comment lines are skipped.
func (l *lexer) continuesLine() bool {
	i := l.off
	for i < l.end {
		c := l.src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '/' && i+1 < l.end && l.src[i+1] == '/':
			for i < l.end && l.src[i] != '\n' {
				i++
			}
		case c == '.':
			return !(i+1 < l.end && l.src[i+1] == '.')
		default:
			return false
		}
	}
	return false
}

func (l *lexer) comment() {
	start := l.pos()
	for l.off < l.end && l.src[l.off] != '\n' {
		l.advance()
	}
	text := string(l.src[start.Offset:l.off])
	doc := strings.HasPrefix(text, "///") && !strings.HasPrefix(text, "////")
	if doc {
		text = text[3:]
	} else {
		text = text[2:]
	}
	text = strings.TrimSuffix(strings.TrimPrefix(text, " "), "\r")
	l.comments = append(l.comments, Comment{Span: l.span(start), Text: text, Doc: doc})
}

func isLetter(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

func (l *lexer) next() {
	start := l.pos()
	c := l.src[l.off]

	// Import path directly after `use`.
	if l.last == Use && !l.rangeMode {
		l.importPath(start)
		return
	}

	switch {
	case c == 'r' && (l.peek(1) == '"' || (l.peek(1) == '#' && l.rawHashes(1) > 0)):
		l.rawString(start)
		return
	case c == 'b' && l.peek(1) == '"':
		l.bytesLit(start)
		return
	case isLetter(c):
		for l.off < l.end && (isLetter(l.src[l.off]) || isDigit(l.src[l.off])) {
			l.advance()
		}
		word := string(l.src[start.Offset:l.off])
		l.emit(Lookup(word), start)
		return
	case isDigit(c):
		l.number(start)
		return
	case c == '"':
		l.stringLit(start)
		return
	case c == '\'':
		l.runeLit(start)
		return
	}

	if k, n := l.operator(); n > 0 {
		l.advanceN(n)
		if k == Newline { // ';'
			l.toks = append(l.toks, Token{Kind: Newline, Span: l.span(start), Text: ";"})
			l.last = Newline
			return
		}
		l.emit(k, start)
		return
	}

	// Invalid character: skip the whole UTF-8 sequence.
	r, size := utf8.DecodeRune(l.src[l.off:l.end])
	l.advanceN(size)
	l.errorf(InvalidChar, l.span(start), "invalid character %q", r)
	l.emit(Illegal, start)
}

// rawHashes counts `#` characters at offset i from the current position and
// returns the count if they are followed by `"`, else 0.
func (l *lexer) rawHashes(i int) int {
	n := 0
	for l.peek(i+n) == '#' {
		n++
	}
	if l.peek(i+n) == '"' {
		return n
	}
	return 0
}

func (l *lexer) operator() (Kind, int) {
	c, c1, c2 := l.peek(0), l.peek(1), l.peek(2)
	switch c {
	case '+':
		if c1 == '=' {
			return PlusAssign, 2
		}
		return Plus, 1
	case '-':
		if c1 == '=' {
			return MinusAssign, 2
		}
		if c1 == '>' {
			return Arrow, 2
		}
		return Minus, 1
	case '*':
		if c1 == '=' {
			return StarAssign, 2
		}
		return Star, 1
	case '/':
		if c1 == '=' {
			return SlashAssign, 2
		}
		return Slash, 1
	case '%':
		if c1 == '=' {
			return PercentAssign, 2
		}
		return Percent, 1
	case '<':
		if c1 == '<' {
			if c2 == '=' {
				return ShlAssign, 3
			}
			return Shl, 2
		}
		if c1 == '=' {
			return LtEq, 2
		}
		return Lt, 1
	case '>':
		if c1 == '>' {
			if c2 == '=' {
				return ShrAssign, 3
			}
			return Shr, 2
		}
		if c1 == '=' {
			return GtEq, 2
		}
		return Gt, 1
	case '&':
		if c1 == '&' {
			return AndAnd, 2
		}
		if c1 == '=' {
			return AmpAssign, 2
		}
		return Amp, 1
	case '|':
		if c1 == '|' {
			return OrOr, 2
		}
		if c1 == '=' {
			return PipeAssign, 2
		}
		return Pipe, 1
	case '^':
		if c1 == '=' {
			return CaretAssign, 2
		}
		return Caret, 1
	case '~':
		return Tilde, 1
	case '!':
		if c1 == '=' {
			return NotEq, 2
		}
		return Bang, 1
	case '?':
		if c1 == '?' {
			if c2 == '?' {
				return Hole, 3
			}
			return Coalesce, 2
		}
		return Question, 1
	case '=':
		if c1 == '=' {
			return Eq, 2
		}
		if c1 == '>' {
			return FatArrow, 2
		}
		return Assign, 1
	case '.':
		if c1 == '.' {
			if c2 == '=' {
				return DotDotEq, 3
			}
			return DotDot, 2
		}
		return Dot, 1
	case ',':
		return Comma, 1
	case ':':
		return Colon, 1
	case ';':
		return Newline, 1
	case '(':
		return LParen, 1
	case ')':
		return RParen, 1
	case '[':
		return LBrack, 1
	case ']':
		return RBrack, 1
	case '{':
		return LBrace, 1
	case '}':
		return RBrace, 1
	case '#':
		if c1 == '{' {
			return HashLBrace, 2
		}
	}
	return Illegal, 0
}

// hasDotSegment reports whether an import path has a `.` or `..` segment.
// Such a path names a package under a second spelling, which would slip
// past `deny` rules that compare paths as written (core 4.1).
func hasDotSegment(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// importPath lexes the path after `use`: segments of letters, digits, `_`,
// `-`, and `.` separated by `/`.
func (l *lexer) importPath(start Pos) {
	isPathChar := func(c byte) bool {
		return isLetter(c) || isDigit(c) || c == '-' || c == '.' || c == '/'
	}
	for l.off < l.end && isPathChar(l.src[l.off]) {
		l.advance()
	}
	if l.off == start.Offset {
		// Not a path at all; lex the token normally so the parser reports it.
		l.last = Illegal
		l.next()
		return
	}
	t := l.emit(ImportPath, start)
	t.Value = t.Text
	if strings.HasPrefix(t.Text, "/") || strings.HasSuffix(t.Text, "/") || strings.Contains(t.Text, "//") {
		l.errorf(MalformedImportPath, t.Span, "malformed import path %q", t.Text)
	} else if hasDotSegment(t.Text) {
		l.errorf(MalformedImportPath, t.Span, "malformed import path %q: `.` and `..` segments are not allowed; import paths are absolute (core 4.1)", t.Text)
	}
}

func (l *lexer) number(start Pos) {
	kind := Int
	base := 10
	if l.src[l.off] == '0' && (l.peek(1)|0x20 == 'x' || l.peek(1)|0x20 == 'o' || l.peek(1)|0x20 == 'b') {
		switch l.peek(1) | 0x20 {
		case 'x':
			base = 16
		case 'o':
			base = 8
		case 'b':
			base = 2
		}
		l.advanceN(2)
		for l.off < l.end && (isHex(l.src[l.off]) || l.src[l.off] == '_') {
			l.advance()
		}
	} else if l.last == Dot {
		// A tuple field index (`t.0`, core 3.5): plain digits only, so
		// `t.0.1` is two field accesses rather than `t.` and `0.1`.
		l.digits()
	} else {
		l.digits()
		if l.peek(0) == '.' && isDigit(l.peek(1)) {
			kind = Float
			l.advance()
			l.digits()
		}
		if l.peek(0)|0x20 == 'e' && (isDigit(l.peek(1)) || ((l.peek(1) == '+' || l.peek(1) == '-') && isDigit(l.peek(2)))) {
			kind = Float
			l.advanceN(2)
			l.digits()
		}
	}
	numEnd := l.off
	// Suffix: a duration unit or an error.
	sufStart := l.off
	for l.off < l.end && (isLetter(l.src[l.off]) || isDigit(l.src[l.off])) {
		l.advance()
	}
	suffix := string(l.src[sufStart:l.off])
	digits := string(l.src[start.Offset:numEnd])

	if suffix != "" {
		t := l.emit(Duration, start)
		unit, ok := durationUnits[suffix]
		switch {
		case !ok:
			l.errorf(InvalidDuration, t.Span, "invalid suffix %q on number literal; duration units are ns, us, ms, s, m, h", suffix)
			return
		case kind != Int || base != 10:
			l.errorf(InvalidDuration, t.Span, "duration literal %q must be a decimal integer followed by a unit", t.Text)
			return
		}
		v, ok := l.parseInt(digits, 10, t.Span)
		if !ok {
			return
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n > (1<<63-1)/unit {
			l.errorf(MalformedNumber, t.Span, "duration literal %q overflows a 64 bit nanosecond count", t.Text)
			return
		}
		t.Value = strconv.FormatInt(n*unit, 10)
		return
	}

	t := l.emit(kind, start)
	if kind == Float {
		if !validUnderscores(digits, 10) {
			l.errorf(MalformedNumber, t.Span, "misplaced `_` in number literal %q", digits)
			return
		}
		t.Value = strings.ReplaceAll(digits, "_", "")
		return
	}
	body := digits
	if base != 10 {
		body = digits[2:]
	}
	if v, ok := l.parseInt(body, base, t.Span); ok {
		t.Value = v
	}
}

var durationUnits = map[string]int64{
	"ns": 1,
	"us": 1_000,
	"ms": 1_000_000,
	"s":  1_000_000_000,
	"m":  60 * 1_000_000_000,
	"h":  3600 * 1_000_000_000,
}

func isHex(c byte) bool { return isDigit(c) || (c|0x20 >= 'a' && c|0x20 <= 'f') }

func (l *lexer) digits() {
	for l.off < l.end && (isDigit(l.src[l.off]) || l.src[l.off] == '_') {
		l.advance()
	}
}

// validUnderscores checks that `_` only separates digits.
func validUnderscores(s string, base int) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			continue
		}
		if i == 0 || i == len(s)-1 {
			return false
		}
		prev, next := s[i-1], s[i+1]
		okDigit := func(c byte) bool {
			if base == 16 {
				return isHex(c)
			}
			return isDigit(c)
		}
		if !okDigit(prev) || !okDigit(next) {
			return false
		}
	}
	return true
}

// parseInt validates digits for base and returns the decimal value.
func (l *lexer) parseInt(body string, base int, sp Span) (string, bool) {
	if body == "" {
		l.errorf(MalformedNumber, sp, "number literal has no digits")
		return "", false
	}
	if base != 10 && strings.HasPrefix(body, "_") {
		body = body[1:] // 0x_ff is allowed, as in Go
	}
	if !validUnderscores(body, base) {
		l.errorf(MalformedNumber, sp, "misplaced `_` in number literal")
		return "", false
	}
	clean := strings.ReplaceAll(body, "_", "")
	if clean == "" {
		l.errorf(MalformedNumber, sp, "number literal has no digits")
		return "", false
	}
	v, err := strconv.ParseUint(clean, base, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			l.errorf(MalformedNumber, sp, "integer literal out of range")
		} else {
			l.errorf(MalformedNumber, sp, "invalid digit in base %d literal", base)
		}
		return "", false
	}
	return strconv.FormatUint(v, 10), true
}

func (l *lexer) rawString(start Pos) {
	l.advance() // r
	hashes := 0
	for l.peek(0) == '#' {
		hashes++
		l.advance()
	}
	l.advance() // "
	bodyStart := l.off
	closing := "\"" + strings.Repeat("#", hashes)
	for l.off < l.end {
		if l.src[l.off] == '"' && strings.HasPrefix(string(l.src[l.off:min(l.end, l.off+len(closing))]), closing) {
			body := string(l.src[bodyStart:l.off])
			l.advanceN(len(closing))
			t := l.emit(RawString, start)
			t.Value = strings.ReplaceAll(body, "\r\n", "\n")
			return
		}
		l.advance()
	}
	t := l.emit(RawString, start)
	t.Value = string(l.src[bodyStart:l.off])
	l.errorf(UnterminatedString, t.Span, "raw string literal not terminated; expected %s", closing)
}

func (l *lexer) bytesLit(start Pos) {
	l.advance() // b
	l.advance() // "
	var raw []byte
	rawStart := l.pos()
	terminated := false
	for l.off < l.end {
		c := l.src[l.off]
		if c == '"' {
			terminated = true
			break
		}
		if c == '\n' {
			break
		}
		if c == '\\' && l.off+1 < l.end {
			raw = append(raw, c, l.src[l.off+1])
			l.advanceN(2)
			continue
		}
		raw = append(raw, c)
		l.advance()
	}
	if terminated {
		l.advance()
	}
	t := l.emit(Bytes, start)
	if !terminated {
		l.errorf(UnterminatedString, t.Span, "bytes literal not terminated")
	}
	t.Value = l.cook(string(raw), rawStart, true)
}

func (l *lexer) runeLit(start Pos) {
	l.advance() // '
	var raw []byte
	rawStart := l.pos()
	terminated := false
	for l.off < l.end {
		c := l.src[l.off]
		if c == '\'' {
			terminated = true
			break
		}
		if c == '\n' {
			break
		}
		if c == '\\' && l.off+1 < l.end {
			raw = append(raw, c, l.src[l.off+1])
			l.advanceN(2)
			continue
		}
		raw = append(raw, c)
		l.advance()
	}
	if terminated {
		l.advance()
	}
	t := l.emit(Rune, start)
	if !terminated {
		l.errorf(InvalidRune, t.Span, "rune literal not terminated")
		return
	}
	v := l.cook(string(raw), rawStart, false)
	if utf8.RuneCountInString(v) != 1 {
		l.errorf(InvalidRune, t.Span, "rune literal must hold exactly one character")
		return
	}
	t.Value = v
}

// cook decodes escape sequences in raw (which starts at pos). With bytes set,
// \xHH may be any byte; in text it must be ASCII.
func (l *lexer) cook(raw string, pos Pos, bytes bool) string {
	if !strings.Contains(raw, "\\") {
		return raw
	}
	var b strings.Builder
	col := pos.Col
	line := pos.Line
	off := pos.Offset
	errAt := func(n int, format string, args ...any) {
		sp := Span{File: l.file, Start: Pos{Offset: off, Line: line, Col: col}, End: Pos{Offset: off + n, Line: line, Col: col + n}}
		l.errorf(InvalidEscape, sp, format, args...)
	}
	for i := 0; i < len(raw); {
		c := raw[i]
		if c != '\\' {
			b.WriteByte(c)
			if c == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			off++
			i++
			continue
		}
		if i+1 >= len(raw) {
			errAt(1, "incomplete escape sequence")
			break
		}
		n := 2
		switch e := raw[i+1]; e {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '0':
			b.WriteByte(0)
		case '\\', '"', '\'', '$':
			b.WriteByte(e)
		case 'x':
			if i+4 <= len(raw) && isHex(raw[i+2]) && isHex(raw[i+3]) {
				v, _ := strconv.ParseUint(raw[i+2:i+4], 16, 8)
				if !bytes && v > 0x7f {
					errAt(4, "\\x escape above \\x7f in a string; use \\u{...}")
				}
				b.WriteByte(byte(v))
				n = 4
			} else {
				errAt(2, "\\x escape needs two hex digits")
			}
		case 'u':
			end := strings.IndexByte(raw[i:], '}')
			if i+2 < len(raw) && raw[i+2] == '{' && end > 3 {
				hex := raw[i+3 : i+end]
				v, err := strconv.ParseUint(hex, 16, 32)
				if err != nil || v > utf8.MaxRune || (v >= 0xD800 && v <= 0xDFFF) || len(hex) > 6 {
					errAt(end+1, "invalid unicode escape \\u{%s}", hex)
				} else {
					b.WriteRune(rune(v))
				}
				n = end + 1
			} else {
				errAt(2, "unicode escape must be written \\u{XXXX}")
			}
		default:
			errAt(2, "unknown escape sequence \\%c", e)
		}
		i += n
		col += n
		off += n
	}
	return b.String()
}

// piece is a raw part of a string literal before escapes are decoded.
type piece struct {
	expr  bool
	raw   string
	start Pos // position of raw in the source
	end   int // end offset (expressions only)
}

func (l *lexer) stringLit(start Pos) {
	triple := l.peek(1) == '"' && l.peek(2) == '"'
	if triple {
		l.advanceN(3)
	} else {
		l.advance()
	}
	var pieces []piece
	cur := piece{start: l.pos()}
	var curRaw []byte
	flush := func() {
		if len(curRaw) > 0 {
			cur.raw = string(curRaw)
			pieces = append(pieces, cur)
		}
		curRaw = nil
		cur = piece{start: l.pos()}
	}
	terminated := false
	for l.off < l.end {
		c := l.src[l.off]
		if c == '"' && (!triple || (l.peek(1) == '"' && l.peek(2) == '"')) {
			terminated = true
			break
		}
		if c == '\n' && !triple {
			break
		}
		if c == '\\' && l.off+1 < l.end && l.src[l.off+1] != '\n' {
			curRaw = append(curRaw, c, l.src[l.off+1])
			l.advanceN(2)
			continue
		}
		if c == '$' && l.peek(1) == '{' {
			flush()
			open := l.pos()
			l.advanceN(2)
			exprStart := l.pos()
			end := l.scanInterp(l.off, triple)
			if end < 0 {
				// Unterminated: consume to end of line or string.
				for l.off < l.end && l.src[l.off] != '\n' && l.src[l.off] != '"' {
					l.advance()
				}
				l.errorf(UnterminatedInterp, l.span(open), "`${` is not closed by `}`")
				cur = piece{start: l.pos()}
				continue
			}
			for l.off < end {
				l.advance()
			}
			text := string(l.src[exprStart.Offset:end])
			l.advance() // }
			if strings.TrimSpace(text) == "" {
				l.errorf(EmptyInterp, l.span(open), "empty interpolation `${}`")
			} else {
				pieces = append(pieces, piece{expr: true, raw: text, start: exprStart, end: end})
			}
			cur = piece{start: l.pos()}
			continue
		}
		if c == '\r' && l.peek(1) == '\n' {
			l.advance()
			continue
		}
		curRaw = append(curRaw, c)
		l.advance()
	}
	flush()
	if terminated {
		if triple {
			l.advanceN(3)
		} else {
			l.advance()
		}
	}
	t := l.emit(String, start)
	t.Triple = triple
	if !terminated {
		l.errorf(UnterminatedString, t.Span, "string literal not terminated")
	}
	if triple {
		pieces = stripIndent(pieces)
	}
	for _, p := range pieces {
		if p.expr {
			t.Parts = append(t.Parts, StrPart{
				IsExpr: true,
				Text:   p.raw,
				Span:   Span{File: l.file, Start: p.start, End: endPos(p.start, p.raw)},
			})
			continue
		}
		cooked := l.cook(p.raw, p.start, false)
		if cooked == "" {
			continue
		}
		t.Parts = append(t.Parts, StrPart{Text: cooked, Span: Span{File: l.file, Start: p.start, End: endPos(p.start, p.raw)}})
	}
}

// endPos computes the position after text that starts at p.
func endPos(p Pos, text string) Pos {
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			p.Line++
			p.Col = 1
		} else {
			p.Col++
		}
		p.Offset++
	}
	return p
}

// scanInterp finds the `}` that closes an interpolation whose expression
// starts at off. It skips nested braces, strings, raw strings, and runes.
// It returns -1 if the interpolation is not closed (or, in a one line
// string, if a newline comes first).
func (l *lexer) scanInterp(off int, multiline bool) int {
	depth := 0
	src := l.src[:l.end]
	for i := off; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\n' && !multiline:
			return -1
		case c == '{':
			depth++
		case c == '}':
			if depth == 0 {
				return i
			}
			depth--
		case c == 'r' && i+1 < len(src) && (src[i+1] == '"' || src[i+1] == '#') && (i == off || !isLetter(src[i-1]) && !isDigit(src[i-1])):
			j := i + 1
			h := 0
			for j < len(src) && src[j] == '#' {
				h++
				j++
			}
			if j >= len(src) || src[j] != '"' {
				continue
			}
			closing := "\"" + strings.Repeat("#", h)
			k := strings.Index(string(src[j+1:]), closing)
			if k < 0 {
				return -1
			}
			i = j + 1 + k + len(closing) - 1
		case c == '"':
			// Nested string; may itself contain interpolations.
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\n' && !multiline {
					return -1
				}
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '$' && j+1 < len(src) && src[j+1] == '{' {
					e := l.scanInterp(j+2, multiline)
					if e < 0 {
						return -1
					}
					j = e + 1
					continue
				}
				j++
			}
			if j >= len(src) {
				return -1
			}
			i = j
		case c == '\'':
			j := i + 1
			for j < len(src) && src[j] != '\'' && src[j] != '\n' {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			i = j
		}
	}
	return -1
}

// stripIndent applies the triple quoted string rules: the line holding the
// opening quotes is dropped when it is blank, the line holding the closing
// quotes is dropped when it is blank, and the common leading indentation of
// the remaining non blank lines is removed. Indentation is measured on the
// raw source, so an interpolation counts as non blank content.
func stripIndent(pieces []piece) []piece {
	const marker = '\x01'
	var flat strings.Builder
	var exprs []piece
	for _, p := range pieces {
		if p.expr {
			flat.WriteByte(marker)
			exprs = append(exprs, p)
		} else {
			flat.WriteString(p.raw)
		}
	}
	lines := strings.Split(flat.String(), "\n")
	keptFirst := true
	if len(lines) > 1 && strings.Trim(lines[0], " \t") == "" {
		lines = lines[1:]
		keptFirst = false
	}
	if len(lines) > 1 && strings.Trim(lines[len(lines)-1], " \t") == "" {
		lines = lines[:len(lines)-1]
	}
	// A first line with content (text right after the opening quotes) is
	// neither measured nor stripped.
	first := 0
	if keptFirst {
		first = 1
	}
	indent := -1
	for _, ln := range lines[first:] {
		if strings.Trim(ln, " \t") == "" {
			continue
		}
		n := len(ln) - len(strings.TrimLeft(ln, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent < 0 {
		indent = 0
	}
	for i, ln := range lines {
		if i < first {
			continue
		}
		if strings.Trim(ln, " \t") == "" {
			lines[i] = ""
		} else {
			lines[i] = ln[indent:]
		}
	}
	joined := strings.Join(lines, "\n")
	// Rebuild pieces. Literal pieces lose their exact source position after
	// stripping; they keep the position of the first original piece, which
	// is only used for escape error locations.
	var out []piece
	var litStart Pos
	if len(pieces) > 0 {
		litStart = pieces[0].start
	}
	segs := strings.Split(joined, string(marker))
	for i, s := range segs {
		if s != "" {
			out = append(out, piece{raw: s, start: litStart})
		}
		if i < len(exprs) {
			out = append(out, exprs[i])
			litStart = endPos(exprs[i].start, exprs[i].raw)
			litStart = Pos{Offset: litStart.Offset + 1, Line: litStart.Line, Col: litStart.Col + 1}
		}
	}
	return out
}
