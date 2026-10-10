// Package parser implements a recursive descent parser for xo (core 15).
//
// Error handling: every error is reported, but at most one per statement,
// declaration, member, or match arm. After a hard error the parser skips to
// the next newline or closing brace at the same nesting depth (statements and
// members) or to the next line that starts a top level declaration in column
// one (declarations), so one mistake never produces a cascade.
package parser

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/lexer"
	"github.com/xo-lang/xo/internal/token"
)

// Mode controls optional parser behavior.
type Mode uint

const (
	// SignatureMode lets function declarations omit their body, as in the
	// signature listings of spec/std.md.
	SignatureMode Mode = 1 << iota
)

// ParseFile parses one source file. It always returns a File, even when
// there are errors. Diagnostics are sorted by position.
func ParseFile(name string, src []byte, mode Mode) (*ast.File, []diag.Diagnostic) {
	var diags diag.List
	lx := lexer.Lex(name, src, &diags)
	p := &parser{file: name, src: src, toks: lx.Tokens, diags: &diags, mode: mode}
	p.collectDocs(lx.Comments)
	p.lexHabits()
	p.tok = p.toks[0]
	f := p.parseFile()
	diag.Sort(diags.Items)
	return f, diags.Items
}

type parser struct {
	file  string
	src   []byte
	toks  []token.Token
	pos   int
	tok   token.Token
	diags *diag.List
	mode  Mode

	// bad is set by a hard error and cleared at the next statement, member,
	// or declaration boundary. While set, further errors are suppressed.
	bad bool
	// noLit disables struct literals (core 1.4) while parsing the head of
	// if, while, for, match, requires, and ensures.
	noLit bool
	// openRange allows `a..` with no upper bound (inside an index).
	openRange bool
	// fnTypeTop is set while parsing a type in a position where a comma
	// cannot follow (a return type or a type alias), so `uses a, b` in a
	// function type can be diagnosed precisely.
	fnTypeTop bool

	docs map[int]*ast.Doc // doc comments keyed by their last line

	// Habit recovery state (habits.go, decision 0087).
	firstDecl token.Pos // start of the first declaration (or its doc comment)
	lineDone  bool      // a foreign top level line was consumed whole
	noArrowFn bool      // `(x) =>` and `x =>` are not arrow functions here (select ops, match guards)
	inImpl    bool      // parsing the methods of a Rust `impl` block
	angle     *angleState
	// pendingDecls are further methods recovered from a Rust `impl` block.
	pendingDecls []*ast.FnDecl
}

// ---------------------------------------------------------------------------
// Token helpers

func (p *parser) next() {
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	p.tok = p.toks[p.pos]
}

func (p *parser) at(k token.Kind) bool { return p.tok.Kind == k }

func (p *parser) peek(n int) token.Token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) atIdent(name string) bool { return p.tok.Kind == token.Ident && p.tok.Text == name }

func (p *parser) got(k token.Kind) bool {
	if p.at(k) {
		p.next()
		return true
	}
	return false
}

func (p *parser) skipNewlines() {
	for p.at(token.Newline) {
		p.next()
	}
}

// peekPastNewlines returns the kind of the first token after any newlines.
func (p *parser) peekPastNewlines() token.Token {
	i := p.pos
	for i < len(p.toks)-1 && p.toks[i].Kind == token.Newline {
		i++
	}
	return p.toks[i]
}

func (p *parser) prevEnd() token.Pos {
	if p.pos == 0 {
		return p.tok.Span.Start
	}
	return p.toks[p.pos-1].Span.End
}

// spanFrom returns the span from start to the end of the previous token.
func (p *parser) spanFrom(start token.Pos) token.Span {
	end := p.prevEnd()
	if end.Offset < start.Offset {
		end = start
	}
	return token.Span{File: p.file, Start: start, End: end}
}

func describe(t token.Token) string {
	switch t.Kind {
	case token.Ident:
		return fmt.Sprintf("identifier `%s`", t.Text)
	case token.Newline:
		if t.Text == ";" {
			return "`;`"
		}
		return "newline"
	case token.EOF:
		return "end of file"
	case token.Int, token.Float, token.Duration, token.Rune, token.String, token.RawString, token.Bytes:
		text := t.Text
		if len(text) > 20 {
			text = text[:17] + "..."
		}
		return fmt.Sprintf("%s %s", t.Kind, text)
	}
	if t.Kind.IsKeyword() {
		return fmt.Sprintf("keyword `%s`", t.Kind)
	}
	return fmt.Sprintf("`%s`", t.Kind)
}

// ---------------------------------------------------------------------------
// Errors

// errorf reports a hard error unless one is already pending for the current
// statement or declaration. It returns nil when suppressed.
func (p *parser) errorf(code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	if p.bad {
		return nil
	}
	p.bad = true
	if p.tok.Kind == token.Illegal && sp.Start == p.tok.Span.Start {
		return nil // the lexer already reported this token
	}
	return p.diags.Errorf(code, sp, format, args...)
}

// softErrorf reports an error the parser recovers from locally; it does not
// suppress later errors or trigger resynchronization.
func (p *parser) softErrorf(code diag.Code, sp token.Span, format string, args ...any) *diag.Diagnostic {
	if p.bad {
		return nil
	}
	return p.diags.Errorf(code, sp, format, args...)
}

func (p *parser) errorExpected(what string) {
	p.errorf(diag.UnexpectedToken, p.tok.Span, "expected %s, found %s", what, describe(p.tok))
}

// expect consumes a token of kind k or reports an error without consuming.
func (p *parser) expect(k token.Kind, what string) token.Token {
	t := p.tok
	if t.Kind != k {
		p.errorExpected(what)
		return token.Token{Kind: token.Illegal, Span: token.Span{File: p.file, Start: t.Span.Start, End: t.Span.Start}}
	}
	p.next()
	return t
}

func (p *parser) expectIdent(what string) *ast.Ident {
	if p.at(token.Ident) {
		id := &ast.Ident{Base: ast.Base{Sp: p.tok.Span}, Name: p.tok.Text}
		p.next()
		return id
	}
	p.errorExpected(what)
	return &ast.Ident{Base: ast.Base{Sp: token.Span{File: p.file, Start: p.tok.Span.Start, End: p.tok.Span.Start}}}
}

// syncStmt skips to the end of the current statement: a newline at the
// current nesting depth (consumed) or a `}` that closes the enclosing block
// (not consumed).
func (p *parser) syncStmt() {
	depth := 0
	for !p.at(token.EOF) {
		switch p.tok.Kind {
		case token.Newline:
			if depth == 0 {
				p.next()
				return
			}
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack:
			if depth > 0 {
				depth--
			}
		case token.RBrace:
			if depth == 0 {
				return
			}
			depth--
		}
		p.next()
	}
}

// topLevelStart reports whether the current token can start a top level
// declaration and sits in column one at the start of a line.
func (p *parser) topLevelStart() bool {
	if p.tok.Span.Start.Col != 1 {
		return false
	}
	switch p.tok.Kind {
	case token.Fn, token.Pub, token.Struct, token.Enum, token.Interface, token.Type,
		token.Const, token.Test, token.Use:
		return true
	case token.Ident:
		// Declarations written with another language's keyword resync too,
		// so one bad declaration does not hide the habits after it.
		return foreignDeclWords[p.tok.Text] && p.peek(1).Kind == token.Ident
	}
	return false
}

// foreignDeclWords start a declaration in another language (habits.go).
var foreignDeclWords = map[string]bool{
	"func": true, "fun": true, "def": true, "function": true, "impl": true, "trait": true,
	"protocol": true, "class": true, "export": true, "async": true, "data": true, "public": true,
}

// syncTop skips to the next line that starts a top level declaration.
func (p *parser) syncTop() {
	p.next()
	for !p.at(token.EOF) && !p.topLevelStart() {
		p.next()
	}
}

// ---------------------------------------------------------------------------
// Doc comments

func (p *parser) collectDocs(comments []lexer.Comment) {
	p.docs = map[int]*ast.Doc{}
	var cur *ast.Doc
	last := -10
	for _, c := range comments {
		if !c.Doc || !p.ownLine(c.Span.Start) {
			cur, last = nil, -10
			continue
		}
		line := c.Span.Start.Line
		if cur != nil && line == last+1 {
			delete(p.docs, last)
			cur.Text += "\n" + c.Text
			cur.Sp.End = c.Span.End
		} else {
			cur = &ast.Doc{Base: ast.Base{Sp: c.Span}, Text: c.Text}
		}
		p.docs[line] = cur
		last = line
	}
}

// ownLine reports whether only blanks precede pos on its line.
func (p *parser) ownLine(pos token.Pos) bool {
	for i := pos.Offset - 1; i >= 0; i-- {
		switch p.src[i] {
		case ' ', '\t':
			continue
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}

// docFor returns the doc comment ending on the line before line.
func (p *parser) docFor(line int) *ast.Doc { return p.docs[line-1] }

// ---------------------------------------------------------------------------
// File and declarations

func (p *parser) parseFile() *ast.File {
	f := &ast.File{Name: p.file}
	start := p.tok.Span.Start
	seenDecl := false
	for {
		p.skipNewlines()
		if p.at(token.EOF) {
			break
		}
		p.bad = false
		doc := p.docFor(p.tok.Span.Start.Line)
		declStart := p.tok.Span.Start
		pub := false
		var pubTok token.Token
		if p.at(token.Pub) {
			pub, pubTok = true, p.tok
			p.next()
		}
		d := p.parseTopDecl(f, doc, pub, pubTok, declStart, seenDecl)
		if d != nil {
			f.Decls = append(f.Decls, d)
			if !seenDecl {
				p.firstDecl = declStart
				if doc != nil {
					p.firstDecl = doc.Sp.Start
				}
			}
			seenDecl = true
		}
		for _, pd := range p.pendingDecls {
			f.Decls = append(f.Decls, pd)
		}
		p.pendingDecls = nil
		if p.bad {
			p.syncTop()
			continue
		}
		if p.lineDone {
			p.lineDone = false
			continue
		}
		if !p.at(token.EOF) && !p.got(token.Newline) {
			p.errorf(diag.ExpectedNewline, p.tok.Span, "expected newline after declaration, found %s", describe(p.tok))
			p.syncTop()
		}
	}
	f.Sp = token.Span{File: p.file, Start: start, End: p.tok.Span.End}
	return f
}

// parseTopDecl parses one top level item after an optional `pub`. It
// returns nil for a `use` (appended to f.Uses) and for skipped foreign
// lines.
func (p *parser) parseTopDecl(f *ast.File, doc *ast.Doc, pub bool, pubTok token.Token, declStart token.Pos, seenDecl bool) ast.Decl {
	var d ast.Decl
	{
		switch p.tok.Kind {
		case token.Use:
			if pub {
				p.softErrorf(diag.MisplacedPub, pubTok.Span, "`use` cannot be `pub`")
			}
			useTok := p.tok
			u := p.parseUse()
			if seenDecl {
				d := p.softErrorf(diag.UseAfterDecl, useTok.Span, "`use` must come before all declarations")
				p.moveUseFix(d, f, u)
			}
			f.Uses = append(f.Uses, u)
		case token.Fn:
			d = p.parseFnDecl(doc, pub, declStart, false)
		case token.Struct:
			d = p.parseStruct(doc, pub, declStart)
		case token.Enum:
			d = p.parseEnum(doc, pub, declStart)
		case token.Interface:
			d = p.parseInterface(doc, pub, declStart)
		case token.Type:
			d = p.parseTypeDecl(doc, pub, declStart)
		case token.Const:
			d = p.parseConst(doc, pub, declStart)
		case token.Test:
			if pub {
				p.softErrorf(diag.MisplacedPub, pubTok.Span, "a test cannot be `pub`")
			}
			d = p.parseTest(doc, declStart)
		default:
			if fd, ok := p.foreignTopLevel(f, doc, pub, pubTok, declStart, seenDecl); ok {
				return fd
			}
			if pub {
				p.errorf(diag.UnexpectedToken, p.tok.Span, "expected a declaration after `pub`, found %s", describe(p.tok))
			} else {
				p.topLevelStmtError()
			}
			d = &ast.BadDecl{Base: ast.Base{Sp: p.tok.Span}}
		}
	}
	return d
}

func (p *parser) topLevelStmtError() {
	switch p.tok.Kind {
	case token.Let, token.Var, token.Return, token.For, token.While, token.If, token.Match,
		token.Defer, token.Break, token.Continue, token.Scope, token.Select,
		token.Ident, token.Int, token.String, token.LParen, token.LBrack, token.LBrace:
		p.errorf(diag.StmtAtTopLevel, p.tok.Span, "statements are only allowed inside a function or test body, found %s", describe(p.tok))
	default:
		p.errorExpected("a declaration")
	}
}

func (p *parser) parseUse() *ast.UseDecl {
	start := p.tok.Span.Start
	p.next() // use
	u := &ast.UseDecl{}
	if p.at(token.ImportPath) {
		u.Path = p.tok.Value
		p.next()
		if u.Path == "go" && (p.at(token.String) || p.at(token.RawString)) {
			// `use go "<Go import path>"` (decision 0041)
			u.Go = true
			t := p.tok
			u.Path = t.Value
			interp := false
			if t.Kind == token.String {
				var b strings.Builder
				for _, part := range t.Parts {
					if part.IsExpr {
						interp = true
					}
					b.WriteString(part.Text)
				}
				u.Path = b.String()
			}
			sp := t.Span
			p.next()
			if interp || u.Path == "" || strings.ContainsAny(u.Path, " \t\n\\") || strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, ".") || strings.HasSuffix(u.Path, "/") {
				p.errorf(diag.MalformedImportPath, sp, "malformed Go import path %q", u.Path)
			}
		} else if u.Path == "c" && (p.at(token.String) || p.at(token.RawString)) {
			// `use c "<header>" [link "<lib>"]...` (decision 0116)
			u.C = true
			h, sp, ok := p.plainString()
			u.Path = h
			if !ok || h == "" || strings.ContainsAny(h, "\t\n\\\"") || strings.HasSuffix(h, "/") {
				p.errorf(diag.MalformedImportPath, sp, "malformed C header path %q", h)
			}
			for p.at(token.Ident) && p.tok.Text == "link" {
				p.next()
				if !p.at(token.String) && !p.at(token.RawString) {
					p.errorf(diag.MalformedImportPath, p.tok.Span, "expected a library after `link`, found %s", describe(p.tok))
					break
				}
				l, lsp, ok := p.plainString()
				if !ok || l == "" || strings.ContainsAny(l, " \t\n\\") {
					p.errorf(diag.MalformedImportPath, lsp, "malformed `link` library %q", l)
				}
				u.Links = append(u.Links, l)
			}
		}
	} else {
		p.errorf(diag.MalformedImportPath, p.tok.Span, "expected an import path after `use`, found %s", describe(p.tok))
	}
	if p.got(token.As) {
		u.Alias = p.expectIdent("a name after `as`")
	}
	if u.C && p.at(token.Uses) {
		p.next()
		u.Uses = []*ast.Effect{}
		if p.at(token.Ident) && p.tok.Text == "none" {
			p.next()
		} else {
			for {
				u.Uses = append(u.Uses, p.parseEffect())
				if !p.got(token.Comma) {
					break
				}
			}
		}
	}
	u.Sp = p.spanFrom(start)
	return u
}

// plainString consumes a string literal without interpolation and
// returns its text, its span, and false when it interpolates.
func (p *parser) plainString() (string, token.Span, bool) {
	t := p.tok
	p.next()
	if t.Kind != token.String {
		return t.Value, t.Span, true
	}
	var b strings.Builder
	ok := true
	for _, part := range t.Parts {
		if part.IsExpr {
			ok = false
		}
		b.WriteString(part.Text)
	}
	return b.String(), t.Span, ok
}

func (p *parser) parseFnDecl(doc *ast.Doc, pub bool, start token.Pos, inInterface bool) *ast.FnDecl {
	fn := &ast.FnDecl{Doc: doc, Pub: pub}
	p.next() // fn
	if p.at(token.LParen) {
		fn.Recv = p.parseReceiver()
	}
	fn.Name = p.expectIdent("a function name")
	if p.atTypeParams() {
		fn.TypeParams = p.parseTypeParams()
	}
	fn.Params = p.parseParams(paramDecl, inInterface || p.inImpl)
	var throwsTok *token.Token
	if !p.bad && p.atIdent("throws") {
		t := p.tok
		throwsTok = &t
		p.next()
	}
	if !p.bad {
		p.foreignResult(fn)
	}
	if !p.bad && p.at(token.Newline) && p.peekPastNewlines().Kind == token.Arrow {
		// A result type on the next line continues the declaration: a line
		// cannot otherwise start with `->` (std.md 7 writes signatures so).
		p.skipNewlines()
	}
	if fn.Result == nil && p.got(token.Arrow) {
		fn.Result = p.parseTypeTop()
	}
	if p.at(token.Bang) {
		fn.Err = p.parseErrSpec()
	}
	if throwsTok != nil {
		p.foreignThrows(fn, *throwsTok)
	}
	fn.Clauses = p.parseClauses()
	if p.peekPastNewlines().Kind == token.LBrace {
		p.skipNewlines()
		if inInterface && !p.bad {
			p.softErrorf(diag.UnexpectedToken, p.tok.Span, "interface method `%s` cannot have a body (core 15: FnSig)", fn.Name.Name)
		}
		fn.Body = p.parseBlock()
	} else if !inInterface && p.mode&SignatureMode == 0 {
		p.errorf(diag.MissingBody, p.tok.Span, "expected `{` to start the body of `%s`, found %s", fn.Name.Name, describe(p.tok))
	}
	fn.Sp = p.spanFrom(start)
	return fn
}

func (p *parser) parseReceiver() *ast.Receiver {
	start := p.tok.Span.Start
	p.next() // (
	r := &ast.Receiver{}
	r.Var = p.got(token.Var)
	r.Name = p.expectIdent("a receiver name")
	if !p.bad && (p.at(token.Star) || p.at(token.Ident)) {
		// Go receivers: `(p *P)` and `(p P)`.
		t := p.tok
		if t.Kind == token.Star {
			d := p.foreign(t.Span, "a pointer receiver `*%s` is Go syntax; an Xo receiver is a value, `(%s: %s)`, and `(var %s: %s)` lets the method change the caller's value (core 3.4)", p.peek(1).Text, r.Name.Name, p.peek(1).Text, r.Name.Name, p.peek(1).Text)
			addFix(d, "write `: "+p.peek(1).Text+"`", p.replaceRange(r.Name.Sp.End, p.peek(1).Span.Start, ": "))
			p.next()
		} else {
			d := p.foreign(t.Span, "`(%s %s)` is Go syntax; Xo writes the receiver type after a colon, `(%s: %s)` (core 3.4)", r.Name.Name, t.Text, r.Name.Name, t.Text)
			addFix(d, "insert `:`", p.replaceRange(r.Name.Sp.End, t.Span.Start, ": "))
		}
		r.Type = p.parseType()
		p.expect(token.RParen, "`)` after the receiver")
		r.Sp = p.spanFrom(start)
		return r
	}
	p.expect(token.Colon, "`:` after the receiver name")
	r.Type = p.parseType()
	p.expect(token.RParen, "`)` after the receiver")
	r.Sp = p.spanFrom(start)
	return r
}

// atTypeParams reports whether a type parameter list starts here: `[` or,
// as a habit, `<` right after the declared name.
func (p *parser) atTypeParams() bool {
	return p.at(token.LBrack) || (p.at(token.Lt) && p.pos > 0 && p.toks[p.pos-1].Kind == token.Ident && adjacent(p.toks[p.pos-1], p.tok))
}

func (p *parser) parseTypeParams() []*ast.TypeParam {
	var out []*ast.TypeParam
	elem := func() {
		start := p.tok.Span.Start
		tp := &ast.TypeParam{Name: p.expectIdent("a type parameter name")}
		if p.atIdent("extends") && p.peek(1).Kind == token.Ident {
			// TypeScript and Kotlin `T extends Bound`.
			d := p.foreign(p.tok.Span, "`%s extends %s` is TypeScript syntax; Xo writes a constraint after a colon, `%s: %s` (core 2.7)", tp.Name.Name, p.peek(1).Text, tp.Name.Name, p.peek(1).Text)
			addFix(d, "write `:`", p.replaceRange(tp.Name.Sp.End, p.peek(1).Span.Start, ": "))
			p.next()
			tp.Constraint = p.parseBounds(tp.Name, p.parseType())
			tp.Sp = p.spanFrom(start)
			out = append(out, tp)
			return
		}
		if p.got(token.Colon) {
			t := p.parseBounds(tp.Name, p.parseType())
			if nt, ok := t.(*ast.NamedType); ok && len(nt.Path) == 1 && nt.Path[0].Name == "Effects" && len(nt.Args) == 0 {
				tp.Effects = true
			} else {
				tp.Constraint = t
			}
		}
		tp.Sp = p.spanFrom(start)
		out = append(out, tp)
	}
	if p.at(token.Lt) {
		p.angleTypeParams(elem)
		return out
	}
	p.parseList(token.LBrack, token.RBrack, "type parameter", elem)
	return out
}

type paramKind int

const (
	paramDecl    paramKind = iota // types required, defaults allowed
	paramClosure                  // types optional, no defaults
)

func (p *parser) parseParams(kind paramKind, allowSelf bool) []*ast.Param {
	var out []*ast.Param
	if !p.at(token.LParen) {
		p.errorExpected("`(` to start the parameter list")
		return nil
	}
	p.parseList(token.LParen, token.RParen, "parameter", func() {
		if n := len(out); n > 0 && p.unparenthesizedEffect(out[n-1]) {
			return
		}
		out = append(out, p.parseParam(kind, allowSelf && len(out) == 0))
	})
	return out
}

// unparenthesizedEffect diagnoses `f: fn() uses a, b, c: T` where `b` was
// meant as a second effect of the function type: a parameter name followed
// directly by `,` or `)` after a parameter whose function type has exactly
// one unparenthesized effect. The name is consumed as an effect.
func (p *parser) unparenthesizedEffect(prev *ast.Param) bool {
	ft, ok := prev.Type.(*ast.FnType)
	if !ok || len(ft.Uses) == 0 || ft.UsesParens || !p.at(token.Ident) {
		return false
	}
	if k := p.peek(1).Kind; k != token.Comma && k != token.RParen && k != token.Dot {
		return false
	}
	first := ft.Uses[0]
	eff := p.parseEffect()
	d := p.softErrorf(diag.FnTypeEffectsParens, first.Sp.To(eff.Sp),
		"several effects in a function type must be parenthesized: `uses (%s, %s)`", effectText(first), effectText(eff))
	if d != nil {
		d.Fixes = append(d.Fixes, diag.Fix{Title: "parenthesize the effect list", Edits: []diag.Edit{
			{Span: diag.PointSpan(p.file, first.Sp.Start), Replace: "("},
			{Span: diag.PointSpan(p.file, eff.Sp.End), Replace: ")"},
		}})
	}
	ft.Uses = append(ft.Uses, eff)
	ft.UsesParens = true // report once per type
	return true
}

func (p *parser) parseParam(kind paramKind, allowSelf bool) *ast.Param {
	start := p.tok.Span.Start
	prm := &ast.Param{}
	if p.foreignSelfParam(prm, allowSelf) {
		prm.Sp = p.spanFrom(start)
		return prm
	}
	prm.Var = p.got(token.Var)
	if kind == paramDecl && p.at(token.Ident) && p.peek(1).Kind == token.Ident && p.peek(2).Kind == token.Colon {
		// Swift argument labels: `_ x: Int`, `from x: Int`.
		lbl := p.tok
		d := p.foreign(lbl.Span, "`%s %s:` is a Swift argument label; Xo has none: callers use the parameter name, `f(%s: v)`, or pass the argument by position (core 3.2)", lbl.Text, p.peek(1).Text, p.peek(1).Text)
		addFix(d, "remove the label", p.deleteToken(lbl))
		p.next()
	}
	prm.Name = p.expectIdent("a parameter name")
	if kind == paramDecl && !p.bad && !p.at(token.Colon) && (p.at(token.Star) || p.at(token.LBrack) || (p.at(token.Ident) && p.tok.Span.Start.Line == prm.Name.Sp.Start.Line)) {
		// Go parameters: `x int`, `p *Point`, `xs []int`.
		t := p.tok
		d := p.foreign(t.Span, "`%s %s` is Go syntax; Xo writes the parameter type after a colon, `%s: %s` (core 3.2)", prm.Name.Name, t.Text, prm.Name.Name, t.Text)
		addFix(d, "insert `:`", p.replaceRange(prm.Name.Sp.End, t.Span.Start, ": "))
		prm.Type = p.parseType()
		prm.Sp = p.spanFrom(start)
		return prm
	}
	if kind == paramDecl && p.at(token.Colon) && p.peek(1).Kind == token.Ident && p.peek(1).Text == "inout" && p.peek(2).Kind == token.Ident {
		// Swift `x: inout Int`.
		io := p.peek(1)
		d := p.foreign(io.Span, "`inout` is Swift syntax; Xo marks an in out parameter with `var` before its name, `var %s: %s`, and callers pass a `var` binding without `&` (core 3.2)", prm.Name.Name, p.peek(2).Text)
		edits := []diag.Edit{p.deleteToken(io)}
		if !prm.Var {
			edits = append(edits, p.insertAt(prm.Name.Sp.Start, "var "))
		}
		addFix(d, "write `var "+prm.Name.Name+"`", edits...)
		prm.Var = true
		p.next() // :
		p.next() // inout
		prm.Type = p.parseType()
		prm.Sp = p.spanFrom(start)
		return prm
	}
	if prm.Name.Name == "self" && !prm.Var && !p.at(token.Colon) && kind == paramDecl {
		if !allowSelf {
			p.softErrorf(diag.MisplacedSelf, prm.Name.Sp, "a bare `self` parameter is only allowed first in an interface method signature")
		}
		prm.Self = true
		prm.Sp = p.spanFrom(start)
		return prm
	}
	if p.got(token.Colon) {
		prm.Type = p.parseType()
	} else if kind == paramDecl {
		p.errorExpected(fmt.Sprintf("`:` and a type after parameter `%s`", prm.Name.Name))
	}
	if kind == paramDecl && p.got(token.Assign) {
		prm.Default = p.parseExpr()
	}
	prm.Sp = p.spanFrom(start)
	return prm
}

func (p *parser) parseErrSpec() *ast.ErrSpec {
	start := p.tok.Span.Start
	p.next() // !
	e := &ast.ErrSpec{}
	if p.startsType() {
		e.Type = p.parseType()
	}
	e.Sp = p.spanFrom(start)
	return e
}

func (p *parser) startsType() bool {
	switch p.tok.Kind {
	case token.Ident, token.LParen, token.Fn:
		return true
	}
	return false
}

// parseClauses parses `uses`, `requires`, and `ensures` clauses, which may
// each start on a new line.
func (p *parser) parseClauses() []ast.Clause {
	var out []ast.Clause
	for {
		switch p.peekPastNewlines().Kind {
		case token.Uses, token.Requires, token.Ensures:
		default:
			return out
		}
		p.skipNewlines()
		start := p.tok.Span.Start
		switch p.tok.Kind {
		case token.Uses:
			p.next()
			effs, parens := p.parseEffectClause()
			out = append(out, &ast.UsesClause{Base: ast.Base{Sp: p.spanFrom(start)}, Effects: effs, Parens: parens})
		case token.Requires:
			p.next()
			c := &ast.RequiresClause{Debug: p.parseDebugFlag()}
			c.Cond = p.parseExprNoLit()
			if p.peekPastNewlines().Kind == token.Else {
				p.skipNewlines()
				p.next()
				c.Else = p.parseExprNoLit()
			}
			c.Sp = p.spanFrom(start)
			out = append(out, c)
		case token.Ensures:
			p.next()
			c := &ast.EnsuresClause{Debug: p.parseDebugFlag()}
			c.Cond = p.parseExprNoLit()
			c.Sp = p.spanFrom(start)
			out = append(out, c)
		}
		if p.bad {
			return out
		}
	}
}

// parseDebugFlag consumes `(debug)` right after requires or ensures.
func (p *parser) parseDebugFlag() bool {
	if p.at(token.LParen) && p.peek(1).Kind == token.Ident && p.peek(1).Text == "debug" && p.peek(2).Kind == token.RParen {
		p.next()
		p.next()
		p.next()
		return true
	}
	return false
}

// parseEffectClause parses the effects after `uses` in a declaration or
// closure: a comma separated list, optionally parenthesized.
func (p *parser) parseEffectClause() ([]*ast.Effect, bool) {
	if p.at(token.LParen) {
		var out []*ast.Effect
		p.parseList(token.LParen, token.RParen, "effect", func() {
			out = append(out, p.parseEffect())
		})
		return out, true
	}
	out := []*ast.Effect{p.parseEffect()}
	for p.at(token.Comma) && p.peek(1).Kind == token.Ident {
		p.next()
		out = append(out, p.parseEffect())
	}
	return out, false
}

func (p *parser) parseEffect() *ast.Effect {
	start := p.tok.Span.Start
	e := &ast.Effect{Name: p.expectIdent("an effect name")}
	if p.at(token.Dot) && p.peek(1).Kind == token.Ident {
		p.next()
		e.Sub = p.expectIdent("a sub effect name")
	}
	e.Sp = p.spanFrom(start)
	return e
}

func (p *parser) parseStruct(doc *ast.Doc, pub bool, start token.Pos) *ast.StructDecl {
	s := &ast.StructDecl{Doc: doc, Pub: pub}
	p.next() // struct
	s.Name = p.expectIdent("a struct name")
	if p.atTypeParams() {
		s.TypeParams = p.parseTypeParams()
	}
	return p.parseStructBody(s, start)
}

// parseStructBody parses the fields, invariant, and derive list of a struct
// whose name and type parameters are already parsed.
func (p *parser) parseStructBody(s *ast.StructDecl, start token.Pos) *ast.StructDecl {
	p.parseMembers("struct field", func() {
		fstart := p.tok.Span.Start
		f := &ast.Field{Doc: p.docFor(fstart.Line)}
		f.Pub = p.got(token.Pub)
		f.Name = p.expectIdent("a field name")
		optional := p.foreignField(f)
		if f.Type == nil {
			p.expect(token.Colon, "`:` after the field name")
			f.Type = p.parseType()
		}
		if optional {
			f.Type = &ast.OptionalType{Base: ast.Base{Sp: f.Type.Span()}, Elem: f.Type}
		}
		if p.got(token.Assign) {
			f.Default = p.parseExpr()
		}
		f.Sp = p.spanFrom(fstart)
		s.Fields = append(s.Fields, f)
	})
	if !p.bad && p.peekPastNewlines().Kind == token.Invariant {
		p.skipNewlines()
		p.next()
		s.Invariant = p.parseExpr()
	}
	if !p.bad {
		s.Derive = p.parseDerive()
	}
	s.Sp = p.spanFrom(start)
	return s
}

// parseDerive parses an optional `derive A, B` after a type body.
func (p *parser) parseDerive() []*ast.Ident {
	t := p.peekPastNewlines()
	if t.Kind != token.Ident || t.Text != "derive" {
		return nil
	}
	p.skipNewlines()
	p.next()
	out := []*ast.Ident{p.expectIdent("a derivable interface name")}
	for p.got(token.Comma) {
		out = append(out, p.expectIdent("a derivable interface name"))
	}
	return out
}

// parseMembers parses `{ member* }` where members are separated by newlines
// or commas, resynchronizing after a bad member.
func (p *parser) parseMembers(what string, member func()) {
	open := p.expect(token.LBrace, "`{`")
	if open.Kind != token.LBrace {
		return
	}
	saved := p.bad
	for {
		p.skipNewlines()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		p.bad = false
		before := p.pos
		member()
		if !p.bad {
			if p.got(token.Comma) || p.got(token.Newline) || p.at(token.RBrace) {
				continue
			}
			p.errorf(diag.UnexpectedToken, p.tok.Span, "expected newline, `,`, or `}` after %s, found %s", what, describe(p.tok))
		}
		p.syncStmt()
		if p.pos == before {
			p.next()
		}
	}
	p.bad = saved
	p.closeBrace(open)
}

func (p *parser) closeBrace(open token.Token) {
	if p.at(token.EOF) {
		d := p.errorf(diag.UnclosedDelimiter, open.Span, "`{` is never closed")
		if d != nil {
			sp := diag.FromSpan(p.tok.Span)
			d.Notes = append(d.Notes, diag.NoteItem{Message: "reached end of file", Span: &sp})
		}
		return
	}
	p.expect(token.RBrace, "`}`")
}

func (p *parser) parseEnum(doc *ast.Doc, pub bool, start token.Pos) *ast.EnumDecl {
	e := &ast.EnumDecl{Doc: doc, Pub: pub}
	p.next() // enum
	e.Name = p.expectIdent("an enum name")
	if p.atTypeParams() {
		e.TypeParams = p.parseTypeParams()
	}
	p.parseMembers("enum variant", func() {
		vstart := p.tok.Span.Start
		v := &ast.Variant{Doc: p.docFor(vstart.Line), Name: p.expectIdent("a variant name")}
		if p.at(token.LParen) {
			v.Fields = []*ast.VariantField{}
			p.parseList(token.LParen, token.RParen, "variant field", func() {
				fstart := p.tok.Span.Start
				vf := &ast.VariantField{}
				if p.atIdent("from") && p.peek(1).Kind != token.Colon {
					p.next()
					vf.From = true
				} else {
					vf.Name = p.expectIdent("a field name or `from`")
					p.expect(token.Colon, "`:` after the variant field name")
				}
				vf.Type = p.parseType()
				vf.Sp = p.spanFrom(fstart)
				v.Fields = append(v.Fields, vf)
			})
		}
		v.Sp = p.spanFrom(vstart)
		e.Variants = append(e.Variants, v)
	})
	if !p.bad {
		e.Derive = p.parseDerive()
	}
	e.Sp = p.spanFrom(start)
	return e
}

func (p *parser) parseInterface(doc *ast.Doc, pub bool, start token.Pos) *ast.InterfaceDecl {
	d := &ast.InterfaceDecl{Doc: doc, Pub: pub}
	p.next() // interface
	d.Name = p.expectIdent("an interface name")
	if p.atTypeParams() {
		d.TypeParams = p.parseTypeParams()
	}
	return p.parseInterfaceBody(d, start)
}

// parseInterfaceBody parses the method signatures of an interface whose
// name and type parameters are already parsed.
func (p *parser) parseInterfaceBody(d *ast.InterfaceDecl, start token.Pos) *ast.InterfaceDecl {
	p.parseMembers("method signature", func() {
		mstart := p.tok.Span.Start
		mdoc := p.docFor(mstart.Line)
		if !p.at(token.Fn) {
			p.errorExpected("a method signature starting with `fn`")
			return
		}
		d.Methods = append(d.Methods, p.parseFnDecl(mdoc, false, mstart, true))
	})
	d.Sp = p.spanFrom(start)
	return d
}

func (p *parser) parseTypeDecl(doc *ast.Doc, pub bool, start token.Pos) ast.Decl {
	d := &ast.TypeDecl{Doc: doc, Pub: pub}
	typeTok := p.tok
	p.next() // type
	d.Name = p.expectIdent("a type name")
	if p.atTypeParams() {
		d.TypeParams = p.parseTypeParams()
	}
	if !p.bad && (p.at(token.Struct) || p.at(token.Interface)) && p.peek(1).Kind == token.LBrace {
		// Go: `type P struct {` and `type I interface {`.
		kw := p.tok
		dg := p.foreign(typeTok.Span.To(kw.Span), "`type %s %s {` is Go syntax; Xo writes `%s %s {` (core 2.2, 2.4)", d.Name.Name, kw.Kind, kw.Kind, d.Name.Name)
		addFix(dg, "write `"+kw.Kind.String()+" "+d.Name.Name+"`", p.replaceSpan(typeTok.Span, kw.Kind.String()), p.replaceRange(p.beforeBlanks(kw.Span.Start), kw.Span.End, ""))
		p.next()
		if kw.Kind == token.Struct {
			return p.parseStructBody(&ast.StructDecl{Doc: doc, Pub: pub, Name: d.Name, TypeParams: d.TypeParams}, start)
		}
		return p.parseInterfaceBody(&ast.InterfaceDecl{Doc: doc, Pub: pub, Name: d.Name, TypeParams: d.TypeParams}, start)
	}
	switch {
	case p.got(token.Assign):
		d.Type = p.parseTypeTop()
	case p.got(token.Struct):
		d.Newtype = true
		p.expect(token.LParen, "`(` after `struct` in a newtype")
		d.Type = p.parseType()
		p.expect(token.RParen, "`)`")
	default:
		p.errorExpected("`=` or `struct(` in a type declaration")
	}
	d.Sp = p.spanFrom(start)
	return d
}

func (p *parser) parseConst(doc *ast.Doc, pub bool, start token.Pos) *ast.ConstDecl {
	c := &ast.ConstDecl{Doc: doc, Pub: pub}
	p.next() // const
	c.Name = p.expectIdent("a constant name")
	if p.got(token.Colon) {
		c.Type = p.parseType()
	}
	p.expect(token.Assign, "`=` in a constant declaration")
	c.Value = p.parseExpr()
	c.Sp = p.spanFrom(start)
	return c
}

func (p *parser) parseTest(doc *ast.Doc, start token.Pos) *ast.TestDecl {
	t := &ast.TestDecl{Doc: doc}
	p.next() // test
	if p.at(token.String) || p.at(token.RawString) {
		t.Name = p.parseString().(*ast.StringLit)
	} else {
		p.errorExpected("a test name string")
	}
	if p.got(token.For) {
		t.For = append(t.For, p.parseParam(paramDecl, false))
		for p.got(token.Comma) {
			t.For = append(t.For, p.parseParam(paramDecl, false))
		}
	}
	if p.atIdent("seed") {
		p.next()
		if p.at(token.Int) {
			t.Seed = &ast.IntLit{Base: ast.Base{Sp: p.tok.Span}, Raw: p.tok.Text, Value: p.tok.Value}
			p.next()
		} else {
			p.errorExpected("an integer seed")
		}
	}
	// A `uses` clause and the body may start on following lines, as for
	// function declarations (core 1.3).
	if p.peekPastNewlines().Kind == token.Uses {
		p.skipNewlines()
		p.next()
		t.Uses, _ = p.parseEffectClause()
	}
	if p.peekPastNewlines().Kind == token.LBrace {
		p.skipNewlines()
		t.Body = p.parseBlock()
	} else {
		p.errorExpected("`{` to start the test body")
	}
	t.Sp = p.spanFrom(start)
	return t
}

// ---------------------------------------------------------------------------
// Lists

// parseList parses `open elem, elem, ... close` with an optional trailing
// comma. A newline where a comma belongs (core 1.3) is reported with a fix
// and otherwise treated as a comma.
func (p *parser) parseList(open, close token.Kind, what string, elem func()) token.Span {
	start := p.tok.Span.Start
	p.expect(open, fmt.Sprintf("`%s`", open))
	savedLit, savedRange := p.noLit, p.openRange
	p.noLit, p.openRange = false, false
	defer func() { p.noLit, p.openRange = savedLit, savedRange }()
	for !p.at(close) && !p.at(token.EOF) {
		before := p.pos
		elem()
		if p.bad || p.pos == before {
			break
		}
		if p.got(token.Comma) {
			continue
		}
		if p.at(token.Newline) {
			nl := p.tok
			d := p.softErrorf(diag.MissingComma, nl.Span, "missing `,` before newline in %s list", what)
			if d != nil {
				d.Notes = append(d.Notes, diag.NoteItem{Message: "a list that spans lines needs a comma after every element, including the last (core 1.3)"})
				d.Fixes = append(d.Fixes, diag.Fix{Title: "insert `,`", Edits: []diag.Edit{{Span: diag.PointSpan(p.file, nl.Span.Start), Replace: ","}}})
			}
			p.skipNewlines()
			continue
		}
		if !p.at(close) {
			p.errorf(diag.UnexpectedToken, p.tok.Span, "expected `,` or `%s` in %s list, found %s", close, what, describe(p.tok))
			break
		}
	}
	end := p.tok.Span.End
	if !p.bad {
		p.expect(close, fmt.Sprintf("`%s`", close))
	} else if p.at(close) {
		p.next()
	}
	return token.Span{File: p.file, Start: start, End: end}
}

// ---------------------------------------------------------------------------
// Types

// parseTypeTop parses a type where no comma may follow (return types and
// alias targets).
func (p *parser) parseTypeTop() ast.Type {
	saved := p.fnTypeTop
	p.fnTypeTop = true
	t := p.parseType()
	p.fnTypeTop = saved
	return t
}

func (p *parser) parseType() ast.Type {
	start := p.tok.Span.Start
	var t ast.Type
	if ft, ok := p.foreignType(); ok {
		return ft
	}
	switch p.tok.Kind {
	case token.Ident:
		nt := &ast.NamedType{Path: []*ast.Ident{p.expectIdent("a type name")}}
		for p.at(token.Dot) && p.peek(1).Kind == token.Ident {
			p.next()
			nt.Path = append(nt.Path, p.expectIdent("a type name"))
		}
		if p.at(token.Lt) && adjacent(p.toks[p.pos-1], p.tok) {
			p.angleArgs(nt)
		} else if p.at(token.LBrack) {
			saved := p.fnTypeTop
			p.fnTypeTop = false
			p.parseList(token.LBrack, token.RBrack, "type argument", func() {
				nt.Args = append(nt.Args, p.parseType())
			})
			p.fnTypeTop = saved
		}
		nt.Sp = p.spanFrom(start)
		t = nt
	case token.LParen:
		var elems []ast.Type
		trailingComma := false
		saved := p.fnTypeTop
		p.fnTypeTop = false
		p.parseList(token.LParen, token.RParen, "tuple type", func() {
			elems = append(elems, p.parseType())
			trailingComma = p.at(token.Comma)
		})
		p.fnTypeTop = saved
		if len(elems) == 1 && !trailingComma {
			t = elems[0] // parenthesized type
		} else {
			t = &ast.TupleType{Base: ast.Base{Sp: p.spanFrom(start)}, Elems: elems}
		}
	case token.Fn:
		t = p.parseFnType()
	default:
		p.errorExpected("a type")
		return &ast.BadType{Base: ast.Base{Sp: p.tok.Span}}
	}
	for p.at(token.Question) {
		p.next()
		t = &ast.OptionalType{Base: ast.Base{Sp: p.spanFrom(start)}, Elem: t}
	}
	return t
}

func (p *parser) parseFnType() ast.Type {
	start := p.tok.Span.Start
	top := p.fnTypeTop
	p.fnTypeTop = false
	defer func() { p.fnTypeTop = top }()
	p.next() // fn
	ft := &ast.FnType{}
	if !p.at(token.LParen) {
		p.errorExpected("`(` in a function type")
		return &ast.BadType{Base: ast.Base{Sp: p.spanFrom(start)}}
	}
	p.parseList(token.LParen, token.RParen, "parameter type", func() {
		pstart := p.tok.Span.Start
		fp := &ast.FnTypeParam{Var: p.got(token.Var)}
		fp.Type = p.parseType()
		fp.Sp = p.spanFrom(pstart)
		ft.Params = append(ft.Params, fp)
	})
	if p.got(token.Arrow) {
		ft.Result = p.parseType()
	}
	if p.at(token.Bang) {
		ft.Err = p.parseErrSpec()
	}
	if p.got(token.Uses) {
		if p.at(token.LParen) {
			ft.UsesParens = true
			p.parseList(token.LParen, token.RParen, "effect", func() {
				ft.Uses = append(ft.Uses, p.parseEffect())
			})
		} else {
			ft.Uses = []*ast.Effect{p.parseEffect()}
			if top && p.at(token.Comma) && p.peek(1).Kind == token.Ident {
				// `fn() uses a, b` where nothing else can follow the type.
				first := ft.Uses[0]
				end := p.peek(1)
				d := p.errorf(diag.FnTypeEffectsParens, first.Sp.To(end.Span),
					"several effects in a function type must be parenthesized: `uses (%s, %s)`", effectText(first), end.Text)
				if d != nil {
					d.Fixes = append(d.Fixes, diag.Fix{Title: "parenthesize the effect list", Edits: []diag.Edit{
						{Span: diag.PointSpan(p.file, first.Sp.Start), Replace: "("},
						{Span: diag.PointSpan(p.file, end.Span.End), Replace: ")"},
					}})
				}
			}
		}
	}
	ft.Sp = p.spanFrom(start)
	return ft
}

func effectText(e *ast.Effect) string {
	if e.Sub != nil {
		return e.Name.Name + "." + e.Sub.Name
	}
	return e.Name.Name
}

// ---------------------------------------------------------------------------
// Statements

func (p *parser) parseBlock() *ast.Block {
	start := p.tok.Span.Start
	open := p.expect(token.LBrace, "`{`")
	b := &ast.Block{}
	if open.Kind != token.LBrace {
		b.Sp = p.spanFrom(start)
		return b
	}
	savedBad, savedLit, savedRange := p.bad, p.noLit, p.openRange
	p.noLit, p.openRange = false, false
	for {
		p.skipNewlines()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		p.bad = false
		before := p.pos
		s := p.parseStmt()
		if s != nil {
			b.Stmts = append(b.Stmts, s)
		}
		if !p.bad {
			if p.got(token.Newline) || p.at(token.RBrace) || p.at(token.EOF) {
				continue
			}
			p.errorf(diag.ExpectedNewline, p.tok.Span, "expected newline or `}` after statement, found %s", describe(p.tok))
		}
		p.syncStmt()
		if p.pos == before {
			p.next()
		}
	}
	p.bad, p.noLit, p.openRange = savedBad, savedLit, savedRange
	p.closeBrace(open)
	b.Sp = p.spanFrom(start)
	return b
}

func (p *parser) parseStmt() ast.Stmt {
	start := p.tok.Span.Start
	if s, ok := p.foreignStmt(); ok {
		return s
	}
	switch p.tok.Kind {
	case token.Let:
		p.next()
		s := &ast.LetStmt{Pattern: p.parsePattern()}
		if p.got(token.Colon) {
			s.Type = p.parseType()
		}
		p.expect(token.Assign, "`=` in `let`")
		s.Value = p.parseExpr()
		s.Sp = p.spanFrom(start)
		return s
	case token.Var:
		p.next()
		s := &ast.VarStmt{Name: p.expectIdent("a variable name")}
		if p.got(token.Colon) {
			s.Type = p.parseType()
		}
		p.expect(token.Assign, "`=` in `var`")
		s.Value = p.parseExpr()
		s.Sp = p.spanFrom(start)
		return s
	case token.Defer:
		p.next()
		s := &ast.DeferStmt{}
		if p.at(token.LBrace) {
			blk := p.parseBlock()
			s.Body = &ast.ExprStmt{Base: blk.Base, X: blk}
		} else {
			s.Body = p.parseSimpleStmt()
		}
		s.Sp = p.spanFrom(start)
		return s
	case token.For:
		return p.parseFor(nil, start)
	case token.While:
		return p.parseWhile(nil, start)
	case token.Ident:
		if p.peek(1).Kind == token.Colon && (p.peek(2).Kind == token.For || p.peek(2).Kind == token.While) {
			label := p.expectIdent("a label")
			p.next() // :
			if p.at(token.For) {
				return p.parseFor(label, start)
			}
			return p.parseWhile(label, start)
		}
	case token.Else:
		p.errorf(diag.ElseOnNewLine, p.tok.Span, "`else` must be on the same line as the closing `}` of the `if`")
		return &ast.BadStmt{Base: ast.Base{Sp: p.tok.Span}}
	case token.Fn:
		if p.peek(1).Kind == token.Ident || (p.peek(1).Kind == token.LParen && p.receiverAhead()) {
			p.errorf(diag.DeclInBlock, p.tok.Span, "functions cannot be declared inside a block; use a closure: `let name = fn(...) { ... }`")
			return &ast.BadStmt{Base: ast.Base{Sp: p.tok.Span}}
		}
	case token.Const:
		// A local constant (core 3.1, 15: Stmt).
		return p.parseConst(nil, false, start)
	case token.Struct, token.Enum, token.Interface, token.Type, token.Use, token.Test, token.Pub:
		if p.peek(1).Kind == token.Dot {
			p.errorf(diag.UnexpectedToken, p.tok.Span, "`%s` is a keyword and cannot be used as a name", p.tok.Kind)
			return &ast.BadStmt{Base: ast.Base{Sp: p.tok.Span}}
		}
		p.errorf(diag.DeclInBlock, p.tok.Span, "`%s` declarations are only allowed at top level", p.tok.Kind)
		return &ast.BadStmt{Base: ast.Base{Sp: p.tok.Span}}
	}
	return p.parseSimpleStmt()
}

// receiverAhead reports whether `fn (` starts a method declaration, which
// has a name after the receiver's closing parenthesis.
func (p *parser) receiverAhead() bool {
	depth := 0
	for i := p.pos + 1; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen:
			depth++
		case token.RParen:
			depth--
			if depth == 0 {
				return i+1 < len(p.toks) && p.toks[i+1].Kind == token.Ident
			}
		case token.Newline, token.EOF, token.LBrace:
			return false
		}
	}
	return false
}

// parseSimpleStmt parses return, break, continue, an assignment, or an
// expression statement. It is used for statements, match and select arm
// bodies, and `defer stmt`.
func (p *parser) parseSimpleStmt() ast.Stmt {
	start := p.tok.Span.Start
	switch p.tok.Kind {
	case token.Return:
		retTok := p.tok
		p.next()
		s := &ast.ReturnStmt{}
		if !p.atStmtEnd() {
			s.Value = p.parseExpr()
			if p.at(token.Comma) && !p.bad {
				s.Value = p.foreignReturnList(s.Value, retTok)
			}
		}
		s.Sp = p.spanFrom(start)
		return s
	case token.Break, token.Continue:
		isBreak := p.at(token.Break)
		p.next()
		var label *ast.Ident
		if p.at(token.Ident) {
			label = p.expectIdent("a label")
		}
		if isBreak {
			return &ast.BreakStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Label: label}
		}
		return &ast.ContinueStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Label: label}
	}
	x := p.parseExpr()
	if st, ok := p.foreignIncDec(x, start); ok {
		return st
	}
	if p.tok.Kind.IsAssign() {
		op := p.tok.Kind
		if !isAssignOp(op) {
			p.errorf(diag.UnexpectedToken, p.tok.Span, "`%s` is not an assignment operator; the compound assignments are `+= -= *= /= %%=` (core 3.5, 15: AssignOp)", op)
			return &ast.BadStmt{Base: ast.Base{Sp: p.spanFrom(start)}}
		}
		p.next()
		s := &ast.AssignStmt{Op: op, Target: x, Value: p.parseExpr()}
		s.Sp = p.spanFrom(start)
		return s
	}
	return &ast.ExprStmt{Base: ast.Base{Sp: x.Span()}, X: x}
}

// isAssignOp reports whether k is one of the assignment operators of core
// 15 (AssignOp). The lexer also produces `&= |= ^= <<= >>=`, which Xo does
// not have.
func isAssignOp(k token.Kind) bool {
	switch k {
	case token.Assign, token.PlusAssign, token.MinusAssign, token.StarAssign, token.SlashAssign, token.PercentAssign:
		return true
	}
	return false
}

func (p *parser) atStmtEnd() bool {
	switch p.tok.Kind {
	case token.Newline, token.RBrace, token.EOF, token.Comma:
		return true
	}
	return false
}

func (p *parser) parseFor(label *ast.Ident, start token.Pos) ast.Stmt {
	if fs, ok := p.foreignFor(label, start); ok {
		return fs
	}
	p.next() // for
	s := &ast.ForStmt{Label: label, Pattern: p.parsePattern()}
	if p.atIdent("of") && !p.bad {
		d := p.foreign(p.tok.Span, "`for x of xs` is TypeScript syntax; Xo writes `for x in xs` (core 3.5)")
		addFix(d, "write `in`", p.replaceSpan(p.tok.Span, "in"))
		p.next()
	} else {
		p.expect(token.In, "`in` in a for loop")
	}
	s.Iter = p.parseExprNoLit()
	s.Body = p.parseCondBlock(s.Iter)
	s.Sp = p.spanFrom(start)
	return s
}

func (p *parser) parseWhile(label *ast.Ident, start token.Pos) ast.Stmt {
	p.next() // while
	s := &ast.WhileStmt{Label: label}
	if !p.at(token.LBrace) {
		s.Cond = p.parseExprNoLit()
	}
	s.Body = p.parseCondBlock(s.Cond)
	s.Sp = p.spanFrom(start)
	return s
}

// parseCondBlock parses the block after a condition, first diagnosing an
// unparenthesized struct literal at the end of the condition (core 1.4).
func (p *parser) parseCondBlock(cond ast.Expr) *ast.Block {
	p.checkStructLitInCond(cond)
	return p.parseBlock()
}

// checkStructLitInCond detects `if p == Point{x: 0} {` and reports it with a
// fix, then skips the literal so the real block parses normally.
func (p *parser) checkStructLitInCond(cond ast.Expr) {
	if cond == nil || !p.at(token.LBrace) || !endsInTypeName(cond) {
		return
	}
	t1, t2, t3 := p.peek(1), p.peek(2), p.peek(3)
	fieldStart := t1.Kind == token.Ident && t2.Kind == token.Colon && t3.Kind != token.For && t3.Kind != token.While
	emptyLit := t1.Kind == token.RBrace && t2.Kind == token.LBrace
	if !fieldStart && !emptyLit {
		return
	}
	// Find the end of the literal.
	depth := 0
	end := p.pos
	for i := p.pos; i < len(p.toks); i++ {
		k := p.toks[i].Kind
		if k == token.LBrace || k == token.HashLBrace {
			depth++
		} else if k == token.RBrace {
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
	}
	litEnd := p.toks[end].Span.End
	what := "literal"
	if _, ok := rightmost(cond).(*ast.IsExpr); ok {
		what = "pattern"
	}
	d := p.errorf(diag.StructLitInCondition, cond.Span().To(p.toks[end].Span),
		"a struct %s in a condition must be parenthesized", what)
	if d != nil {
		d.Fixes = append(d.Fixes, diag.Fix{Title: "parenthesize the condition", Edits: []diag.Edit{
			{Span: diag.PointSpan(p.file, cond.Span().Start), Replace: "("},
			{Span: diag.PointSpan(p.file, litEnd), Replace: ")"},
		}})
	}
	for p.pos <= end && !p.at(token.EOF) {
		p.next()
	}
}

// endsInTypeName reports whether a condition ends in a type name that a `{`
// could continue as a struct literal or struct pattern: `p == Point` or
// `p is Point`.
func endsInTypeName(cond ast.Expr) bool {
	r := rightmost(cond)
	if is, ok := r.(*ast.IsExpr); ok {
		v, ok := is.Pattern.(*ast.VariantPat)
		return ok && !v.Parens && isUpper(v.Path[len(v.Path)-1].Name)
	}
	return isTypeName(r)
}

// rightmost returns the last operand of a binary or unary chain.
func rightmost(e ast.Expr) ast.Expr {
	for {
		switch x := e.(type) {
		case *ast.BinaryExpr:
			e = x.Y
		case *ast.UnaryExpr:
			e = x.X
		default:
			return e
		}
	}
}

// ---------------------------------------------------------------------------
// Expressions

func (p *parser) parseExprNoLit() ast.Expr {
	saved := p.noLit
	p.noLit = true
	e := p.parseExpr()
	p.noLit = saved
	return e
}

func (p *parser) parseExpr() ast.Expr {
	x := p.parseBinary(1)
	if !p.bad && p.at(token.Question) && p.ternaryAhead() {
		return p.ternary(x)
	}
	if !p.bad && p.pyCondAhead() {
		return p.pyCond(x)
	}
	return x
}

// Binary operator precedence, lowest first (core 15).
func precedence(k token.Kind) int {
	switch k {
	case token.Coalesce:
		return 1
	case token.OrOr:
		return 2
	case token.AndAnd:
		return 3
	case token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		return 4
	case token.DotDot, token.DotDotEq:
		return 5
	case token.Plus, token.Minus, token.Pipe, token.Caret:
		return 6
	case token.Star, token.Slash, token.Percent, token.Shl, token.Shr, token.Amp:
		return 7
	}
	return 0
}

const rangePrec = 5

// isPrec is the precedence of the contextual `is` operator: that of `==`
// (core 3.5, 15).
const isPrec = 4

func (p *parser) parseBinary(minPrec int) ast.Expr {
	x := p.parseUnary()
	for {
		if p.atIdent("is") && isPrec >= minPrec && !p.bad {
			x = p.parseIs(x)
			continue
		}
		if y, ok := p.foreignBinary(x, minPrec); ok {
			x = y
			continue
		}
		op := p.tok.Kind
		prec := precedence(op)
		if prec == 0 || prec < minPrec || p.bad || p.incDecAhead() {
			return x
		}
		opTok := p.tok
		p.next()
		if prec == rangePrec {
			r := &ast.RangeExpr{Lo: x, Inclusive: op == token.DotDotEq}
			if op == token.DotDot && adjacent(opTok, p.tok) && (p.at(token.Lt) || p.at(token.Dot)) {
				r.Inclusive = p.foreignRange(opTok)
			}
			if !(p.openRange && p.at(token.RBrack)) {
				r.Hi = p.parseBinary(prec + 1)
			}
			r.Sp = p.spanFrom(x.Span().Start)
			x = r
			continue
		}
		var y ast.Expr
		if op == token.Coalesce {
			y = p.parseBinary(prec) // right associative
		} else {
			y = p.parseBinary(prec + 1)
		}
		x = &ast.BinaryExpr{Base: ast.Base{Sp: x.Span().To(y.Span())}, Op: op, X: x, Y: y}
	}
}

// parseIs parses `x is Pattern` after x (core 3.5). The pattern may not
// bind names; a snake_case name in it is reported with a fix to `_`.
func (p *parser) parseIs(x ast.Expr) ast.Expr {
	p.next() // is
	pat := p.parsePattern()
	e := &ast.IsExpr{Base: ast.Base{Sp: x.Span().To(pat.Span())}, X: x, Pattern: pat}
	if p.bad {
		return e
	}
	inspectPattern(pat, func(q ast.Pattern) {
		bp, ok := q.(*ast.BindPat)
		if !ok {
			return
		}
		d := p.softErrorf(diag.InvalidPattern, bp.Sp, "a pattern after `is` cannot bind names, found `%s`; write `_`, or bind with `if let` or `match` (core 3.5)", bp.Name.Name)
		if d != nil {
			d.Fixes = append(d.Fixes, diag.Fix{Title: "replace with `_`", Edits: []diag.Edit{{Span: diag.FromSpan(bp.Sp), Replace: "_"}}})
		}
	})
	return e
}

// inspectPattern calls f for pat and every pattern nested in it.
func inspectPattern(pat ast.Pattern, f func(ast.Pattern)) {
	if pat == nil {
		return
	}
	f(pat)
	switch x := pat.(type) {
	case *ast.OrPat:
		for _, a := range x.Alts {
			inspectPattern(a, f)
		}
	case *ast.TuplePat:
		for _, e := range x.Elems {
			inspectPattern(e, f)
		}
	case *ast.ListPat:
		for _, e := range x.Elems {
			inspectPattern(e, f)
		}
	case *ast.VariantPat:
		for _, fl := range x.Fields {
			inspectPattern(fl.Pattern, f)
		}
	case *ast.StructPat:
		for _, fl := range x.Fields {
			inspectPattern(fl.Pattern, f)
		}
	}
}

func (p *parser) parseUnary() ast.Expr {
	if x, ok := p.foreignUnary(); ok {
		return x
	}
	switch p.tok.Kind {
	case token.Minus, token.Bang, token.Tilde:
		start := p.tok.Span.Start
		op := p.tok.Kind
		p.next()
		x := p.parseUnary()
		return &ast.UnaryExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Op: op, X: x}
	}
	return p.parsePostfix(p.parsePrimary())
}

func (p *parser) parsePrimary() ast.Expr {
	if x, ok := p.foreignPrimary(); ok {
		return x
	}
	t := p.tok
	sp := ast.Base{Sp: t.Span}
	switch t.Kind {
	case token.Ident:
		p.next()
		return &ast.Ident{Base: sp, Name: t.Text}
	case token.Int:
		p.next()
		return &ast.IntLit{Base: sp, Raw: t.Text, Value: t.Value}
	case token.Float:
		p.next()
		return &ast.FloatLit{Base: sp, Raw: t.Text}
	case token.Duration:
		p.next()
		return &ast.DurationLit{Base: sp, Raw: t.Text, Nanos: t.Value}
	case token.Rune:
		p.next()
		r, _ := firstRune(t.Value)
		return &ast.RuneLit{Base: sp, Raw: t.Text, Value: r}
	case token.String, token.RawString:
		return p.parseString()
	case token.Bytes:
		p.next()
		return &ast.BytesLit{Base: sp, Value: t.Value}
	case token.Hole:
		p.next()
		return &ast.HoleExpr{Base: sp}
	case token.LParen:
		return p.parseParenOrTuple()
	case token.LBrack:
		var elems []ast.Expr
		var comp *comprehension
		lsp := p.parseList(token.LBrack, token.RBrack, "list", func() {
			elems = append(elems, p.parseExpr())
			if len(elems) == 1 && p.at(token.For) && !p.bad {
				comp = p.parseComprehension(elems[0])
			}
		})
		if comp != nil {
			return p.finishComprehension(comp, lsp)
		}
		return &ast.ListLit{Base: ast.Base{Sp: lsp}, Elems: elems}
	case token.HashLBrace:
		var elems []ast.Expr
		ssp := p.parseList(token.HashLBrace, token.RBrace, "set", func() {
			elems = append(elems, p.parseExpr())
		})
		return &ast.SetLit{Base: ast.Base{Sp: ssp}, Elems: elems}
	case token.LBrace:
		if p.braceIsMap() {
			return p.parseMapLit()
		}
		return p.parseBlock()
	case token.If:
		return p.parseIf()
	case token.Match:
		return p.parseMatch()
	case token.Scope:
		start := t.Span.Start
		p.next()
		s := &ast.ScopeExpr{Name: p.expectIdent("a scope name")}
		s.Body = p.parseBlock()
		s.Sp = p.spanFrom(start)
		return s
	case token.Select:
		return p.parseSelect()
	case token.Fn:
		return p.parseFnLit()
	case token.While:
		start := t.Span.Start
		p.next()
		if !p.at(token.LBrace) {
			p.errorf(diag.WhileCondAsExpr, t.Span, "a `while` loop with a condition is a statement, not an expression; only `while { ... }` is an expression")
			p.parseExprNoLit()
		}
		body := p.parseBlock()
		return &ast.WhileExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Body: body}
	case token.Return, token.Break, token.Continue:
		p.errorf(diag.UnexpectedToken, t.Span, "`%s` is a statement and cannot be used as a value here", t.Kind)
		return &ast.BadExpr{Base: sp}
	}
	p.errorExpected("an expression")
	return &ast.BadExpr{Base: ast.Base{Sp: token.Span{File: p.file, Start: t.Span.Start, End: t.Span.Start}}}
}

func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func (p *parser) parseParenOrTuple() ast.Expr {
	start := p.tok.Span.Start
	if p.peek(1).Kind == token.RParen {
		p.next()
		p.next()
		return &ast.UnitExpr{Base: ast.Base{Sp: p.spanFrom(start)}}
	}
	var elems []ast.Expr
	trailingComma := false
	sp := p.parseList(token.LParen, token.RParen, "tuple", func() {
		elems = append(elems, p.parseExpr())
		trailingComma = p.at(token.Comma)
	})
	if len(elems) == 1 && !trailingComma {
		return &ast.ParenExpr{Base: ast.Base{Sp: sp}, X: elems[0]}
	}
	return &ast.TupleExpr{Base: ast.Base{Sp: sp}, Elems: elems}
}

// braceIsMap decides whether `{` in expression position starts a map literal
// or a block: `{}` is an empty map (core 1.6), and a map has a `:` at depth
// zero before the first newline. Blocks start with a statement, which never
// contains a top level `:` other than a loop label.
func (p *parser) braceIsMap() bool {
	i := p.pos + 1
	for p.toks[i].Kind == token.Newline {
		i++
	}
	switch p.toks[i].Kind {
	case token.RBrace:
		return true
	case token.Let, token.Var, token.Return, token.Break, token.Continue, token.Defer, token.For, token.While:
		return false
	case token.Ident:
		if p.toks[i+1].Kind == token.Colon && (p.toks[i+2].Kind == token.For || p.toks[i+2].Kind == token.While) {
			return false
		}
	}
	depth := 0
	for ; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack:
			depth--
		case token.RBrace:
			if depth == 0 {
				return false
			}
			depth--
		case token.Newline:
			if depth == 0 {
				return false
			}
		case token.Colon:
			if depth == 0 {
				return true
			}
		case token.EOF:
			return false
		}
	}
	return false
}

func (p *parser) parseMapLit() ast.Expr {
	m := &ast.MapLit{}
	m.Sp = p.parseList(token.LBrace, token.RBrace, "map", func() {
		start := p.tok.Span.Start
		e := &ast.MapEntry{Key: p.parseExpr()}
		p.expect(token.Colon, "`:` between map key and value")
		e.Value = p.parseExpr()
		e.Sp = p.spanFrom(start)
		m.Entries = append(m.Entries, e)
	})
	return m
}

func (p *parser) parseFieldInits() ([]*ast.FieldInit, token.Span) {
	var out []*ast.FieldInit
	sp := p.parseList(token.LBrace, token.RBrace, "field", func() {
		start := p.tok.Span.Start
		f := &ast.FieldInit{Name: p.expectIdent("a field name")}
		p.expect(token.Colon, "`:` after the field name")
		f.Value = p.parseExpr()
		f.Sp = p.spanFrom(start)
		out = append(out, f)
	})
	return out, sp
}

func (p *parser) parseIf() ast.Expr {
	start := p.tok.Span.Start
	p.next() // if
	e := &ast.IfExpr{}
	var head ast.Expr
	if p.got(token.Let) {
		e.Pattern = p.parsePattern()
		p.expect(token.Assign, "`=` in `if let`")
		e.Value = p.parseExprNoLit()
		head = e.Value
	} else {
		e.Cond = p.parseExprNoLit()
		head = e.Cond
	}
	if _, paren := head.(*ast.ParenExpr); paren && !p.bad && !p.at(token.LBrace) && !p.at(token.Newline) && startsExprOrStmt(p.tok.Kind) {
		return p.bracelessIf(e, start)
	}
	e.Then = p.parseCondBlock(head)
	// `}` newline `else` is a common mistake; report it and keep going.
	if p.at(token.Newline) && p.peekPastNewlines().Kind == token.Else {
		nl := p.tok
		elseTok := p.peekPastNewlines()
		d := p.softErrorf(diag.ElseOnNewLine, elseTok.Span, "`else` must be on the same line as the closing `}` (core 1.3)")
		if d != nil {
			d.Fixes = append(d.Fixes, diag.Fix{Title: "join `} else`", Edits: []diag.Edit{{
				Span:    diag.Span{File: p.file, Line: nl.Span.Start.Line, Col: nl.Span.Start.Col, EndLine: elseTok.Span.Start.Line, EndCol: elseTok.Span.Start.Col},
				Replace: " ",
			}}})
		}
		p.skipNewlines()
	}
	if !p.bad && p.foreignElif() {
		e.Else = p.parseIf()
	} else if p.got(token.Else) {
		switch {
		case p.at(token.If):
			e.Else = p.parseIf()
		case p.at(token.LBrace):
			e.Else = p.parseBlock()
		default:
			p.errorExpected("`if` or `{` after `else`")
		}
	}
	e.Sp = p.spanFrom(start)
	return e
}

func (p *parser) parseMatch() ast.Expr {
	start := p.tok.Span.Start
	p.next() // match
	m := &ast.MatchExpr{Subject: p.parseExprNoLit()}
	p.checkStructLitInCond(m.Subject)
	p.parseArms("match arm", func() {
		astart := p.tok.Span.Start
		arm := &ast.MatchArm{Pattern: p.parsePattern()}
		if p.got(token.If) {
			saved := p.noArrowFn
			p.noArrowFn = true
			arm.Guard = p.parseExpr()
			p.noArrowFn = saved
		}
		if p.at(token.Arrow) && !p.bad {
			d := p.foreign(p.tok.Span, "`->` after a pattern is Kotlin and Scala syntax; an Xo match arm is `pattern => body` (core 3.5)")
			addFix(d, "write `=>`", p.replaceSpan(p.tok.Span, "=>"))
			p.next()
		} else {
			p.expect(token.FatArrow, "`=>` after the match pattern")
		}
		if !p.bad {
			arm.Body = p.parseSimpleStmt()
		}
		arm.Sp = p.spanFrom(astart)
		m.Arms = append(m.Arms, arm)
	})
	m.Sp = p.spanFrom(start)
	return m
}

// parseArms parses `{ arm* }` with arms separated by newlines.
func (p *parser) parseArms(what string, arm func()) {
	open := p.expect(token.LBrace, "`{`")
	if open.Kind != token.LBrace {
		return
	}
	savedBad, savedLit, savedRange := p.bad, p.noLit, p.openRange
	p.noLit, p.openRange = false, false
	for {
		p.skipNewlines()
		if p.at(token.RBrace) || p.at(token.EOF) {
			break
		}
		p.bad = false
		before := p.pos
		arm()
		if !p.bad {
			if p.got(token.Newline) || p.at(token.RBrace) {
				continue
			}
			if p.at(token.Comma) {
				p.armComma(what)
				continue
			}
			if p.armOnSameLine() {
				p.splitArm(what)
				continue
			}
			p.errorf(diag.ExpectedNewline, p.tok.Span, "expected newline after %s, found %s", what, describe(p.tok))
		}
		p.syncStmt()
		if p.pos == before {
			p.next()
		}
	}
	p.bad, p.noLit, p.openRange = savedBad, savedLit, savedRange
	p.closeBrace(open)
}

func (p *parser) parseSelect() ast.Expr {
	start := p.tok.Span.Start
	p.next() // select
	s := &ast.SelectExpr{}
	p.parseArms("select arm", func() {
		astart := p.tok.Span.Start
		arm := &ast.SelectArm{}
		savedArrow := p.noArrowFn
		p.noArrowFn = true
		switch {
		case p.atIdent("after") && !isArmOpFollower(p.peek(1).Kind):
			p.next()
			arm.After = p.parseExpr()
		case p.at(token.Ident) && p.peek(1).Kind == token.Assign:
			arm.Bind = p.expectIdent("a name")
			p.next() // =
			arm.Op = p.parseExpr()
		default:
			arm.Op = p.parseExpr()
		}
		p.noArrowFn = savedArrow
		p.expect(token.FatArrow, "`=>` after the select operation")
		if !p.bad {
			arm.Body = p.parseSimpleStmt()
		}
		arm.Sp = p.spanFrom(astart)
		s.Arms = append(s.Arms, arm)
	})
	s.Sp = p.spanFrom(start)
	return s
}

// isArmOpFollower reports whether k after the word `after` shows that
// `after` is an ordinary name rather than the timeout keyword.
func isArmOpFollower(k token.Kind) bool {
	switch k {
	case token.Assign, token.FatArrow, token.Dot, token.LParen, token.LBrack, token.Question:
		return true
	}
	return false
}

func (p *parser) parseFnLit() ast.Expr {
	start := p.tok.Span.Start
	p.next() // fn
	f := &ast.FnLit{}
	f.Params = p.parseParams(paramClosure, false)
	if p.got(token.Arrow) {
		f.Result = p.parseType()
	}
	if p.at(token.Bang) {
		f.Err = p.parseErrSpec()
	}
	if p.got(token.Uses) {
		f.Uses, _ = p.parseEffectClause()
	}
	f.Body = p.parseBlock()
	f.Sp = p.spanFrom(start)
	return f
}

func (p *parser) parsePostfix(x ast.Expr) ast.Expr {
	for !p.bad {
		start := x.Span().Start
		if y, ok, stop := p.foreignPostfix(x); stop {
			return x
		} else if ok {
			x = y
			continue
		}
		switch p.tok.Kind {
		case token.Dot:
			p.next()
			if p.at(token.Int) {
				// tuple field `t.0` (core 3.5)
				t := p.tok
				p.next()
				if t.Value != t.Text {
					p.errorf(diag.UnexpectedToken, t.Span, "a tuple field is a plain decimal index such as `.0`, found `%s`", t.Text)
				}
				sel := &ast.Ident{Base: ast.Base{Sp: t.Span}, Name: t.Text}
				x = &ast.SelectorExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x, Sel: sel}
				continue
			}
			sel := p.expectIdent("a field or method name after `.`")
			x = &ast.SelectorExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x, Sel: sel}
		case token.LParen:
			var args []*ast.Arg
			p.parseList(token.LParen, token.RParen, "argument", func() {
				astart := p.tok.Span.Start
				a := &ast.Arg{}
				if p.at(token.Ident) && p.peek(1).Kind == token.Colon {
					a.Name = p.expectIdent("an argument name")
					p.next()
				}
				if p.at(token.Var) && !p.bad && startsExpr(p.peek(1).Kind) {
					// Swift `inout` call sites and C# `ref` read like this.
					vt := p.tok
					d := p.foreign(vt.Span, "`var` at a call site is not Xo: `var` belongs only on the parameter, `fn f(var x: T)`, and the caller passes its `var` binding as is, `f(x)` (core 3.2)")
					addFix(d, "remove `var`", p.deleteToken(vt))
					p.next()
				}
				a.Value = p.parseExpr()
				a.Sp = p.spanFrom(astart)
				args = append(args, a)
			})
			x = &ast.CallExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Fun: x, Args: args}
		case token.LBrack:
			if p.typeArgsAhead(x) {
				var types []ast.Type
				p.parseList(token.LBrack, token.RBrack, "type argument", func() {
					types = append(types, p.parseType())
				})
				x = &ast.TypeArgsExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x, Types: types}
			} else {
				x = p.parseIndex(x)
			}
		case token.Question:
			p.next()
			x = &ast.TryExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x}
		case token.Ident:
			if p.tok.Text != "with" || p.peek(1).Kind != token.LBrace {
				return x
			}
			p.next()
			fields, _ := p.parseFieldInits()
			x = &ast.WithExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x, Fields: fields}
		case token.LBrace:
			if p.noLit || !isTypeName(x) {
				return x
			}
			fields, _ := p.parseFieldInits()
			x = &ast.StructLit{Base: ast.Base{Sp: p.spanFrom(start)}, Type: x, Fields: fields}
		default:
			return x
		}
	}
	return x
}

func (p *parser) parseIndex(x ast.Expr) ast.Expr {
	start := x.Span().Start
	p.next() // [
	savedLit, savedRange := p.noLit, p.openRange
	p.noLit, p.openRange = false, true
	var idx ast.Expr
	if p.at(token.DotDot) || p.at(token.DotDotEq) {
		rstart := p.tok.Span.Start
		r := &ast.RangeExpr{Inclusive: p.at(token.DotDotEq)}
		p.next()
		if !p.at(token.RBrack) {
			r.Hi = p.parseBinary(rangePrec + 1)
		}
		r.Sp = p.spanFrom(rstart)
		idx = r
	} else if p.at(token.Colon) {
		idx = p.foreignSlice(nil)
	} else if p.at(token.RBrack) {
		p.errorExpected("an index or range")
		idx = &ast.BadExpr{Base: ast.Base{Sp: p.tok.Span}}
	} else {
		idx = p.parseExpr()
		if p.at(token.Colon) && !p.bad {
			idx = p.foreignSlice(idx)
		}
	}
	p.noLit, p.openRange = savedLit, savedRange
	p.expect(token.RBrack, "`]`")
	return &ast.IndexExpr{Base: ast.Base{Sp: p.spanFrom(start)}, X: x, Index: idx}
}

// isTypeName reports whether e names a type: an UpperCamel identifier, a
// selector ending in one (`http.Request`), or either with type arguments.
// Only such expressions may be followed by a struct literal body.
func isTypeName(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return isUpper(x.Name)
	case *ast.SelectorExpr:
		return isUpper(x.Sel.Name) && isPath(x.X)
	case *ast.TypeArgsExpr:
		return isTypeName(x.X)
	}
	return false
}

func isPath(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return isPath(x.X)
	}
	return false
}

func isUpper(s string) bool { return s != "" && s[0] >= 'A' && s[0] <= 'Z' }

// typeArgsAhead decides whether the `[` at the current token starts explicit
// type arguments rather than an index. The brackets hold type arguments when
// every element looks like a type (starts with an UpperCamel name, a module
// qualified UpperCamel name, `(`, or `fn`, and contains only tokens that can
// appear in a type) or like an effect argument (`net`, `net.dial`), and
// either the base is itself a type name (`Mutex[Int].new(0)`,
// `Job[Str]{...}`, `Cache[Int, Item, net.dial].new()`) or a call follows and
// at least one element is a type (`json.decode[User](b)`). So `xs[i]`,
// `xs[0]`, `m[Red]`, and `fns[i](x)` are indexes, and `f[T](x)` is a call.
func (p *parser) typeArgsAhead(base ast.Expr) bool {
	depth := 0
	end := -1
	elemStarts := []int{p.pos + 1}
scan:
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LBrack, token.LParen:
			depth++
		case token.RBrack, token.RParen:
			depth--
			if depth == 0 {
				if p.toks[i].Kind != token.RBrack {
					return false
				}
				end = i
				break scan
			}
		case token.Comma:
			if depth == 1 {
				elemStarts = append(elemStarts, i+1)
			}
		case token.Ident, token.Dot, token.Question, token.Arrow, token.Bang, token.Fn, token.Uses, token.Var:
		default:
			return false
		}
	}
	if end <= p.pos+1 {
		return false
	}
	anyType := false
	for _, s := range elemStarts {
		if s >= end {
			continue // trailing comma
		}
		switch {
		case p.typeStartAt(s):
			anyType = true
		case p.effectArgAt(s):
		default:
			return false
		}
	}
	if isTypeName(base) {
		return true
	}
	return anyType && p.toks[end+1].Kind == token.LParen
}

// effectArgAt reports whether the tokens at i are a whole effect argument
// (core 15: TypeArg): `name` or `name.sub` followed by `,` or `]`.
func (p *parser) effectArgAt(i int) bool {
	if i >= len(p.toks) || p.toks[i].Kind != token.Ident || isUpper(p.toks[i].Text) {
		return false
	}
	j := i + 1
	if j+1 < len(p.toks) && p.toks[j].Kind == token.Dot && p.toks[j+1].Kind == token.Ident {
		j += 2
	}
	return j < len(p.toks) && (p.toks[j].Kind == token.Comma || p.toks[j].Kind == token.RBrack)
}

// typeStartAt reports whether the tokens at i start something that looks
// like a type.
func (p *parser) typeStartAt(i int) bool {
	t := p.toks[i]
	switch t.Kind {
	case token.LParen, token.Fn:
		return true
	case token.Ident:
		if isUpper(t.Text) {
			return true
		}
		// module qualified: mod.Type
		for i+2 < len(p.toks) && p.toks[i+1].Kind == token.Dot && p.toks[i+2].Kind == token.Ident {
			i += 2
			if isUpper(p.toks[i].Text) {
				return true
			}
		}
	}
	return false
}

// parseString builds a StringLit, parsing interpolated expressions.
func (p *parser) parseString() ast.Expr {
	t := p.tok
	p.next()
	s := &ast.StringLit{Base: ast.Base{Sp: t.Span}}
	if t.Kind == token.RawString {
		s.Kind = ast.RawString
		s.Parts = []ast.StringPart{{Text: t.Value}}
		return s
	}
	if t.Triple {
		s.Kind = ast.TripleString
	}
	for _, part := range t.Parts {
		if !part.IsExpr {
			s.Parts = append(s.Parts, ast.StringPart{Text: part.Text})
			continue
		}
		s.Parts = append(s.Parts, ast.StringPart{Expr: p.parseInterp(part)})
	}
	return s
}

// parseInterp lexes and parses the source of one `${...}`.
func (p *parser) parseInterp(part token.StrPart) ast.Expr {
	toks := lexer.LexRange(p.file, p.src, part.Span.Start, part.Span.End.Offset, p.diags)
	sub := &parser{file: p.file, src: p.src, toks: toks, diags: p.diags, mode: p.mode, docs: p.docs}
	sub.tok = toks[0]
	e := sub.parseExpr()
	if !sub.bad && !sub.at(token.EOF) {
		sub.errorf(diag.UnexpectedToken, sub.tok.Span, "unexpected %s in interpolation; expected `}`", describe(sub.tok))
	}
	return e
}

// ---------------------------------------------------------------------------
// Patterns

func (p *parser) parsePattern() ast.Pattern {
	start := p.tok.Span.Start
	first := p.parsePatAlt()
	if !p.at(token.Pipe) {
		return first
	}
	or := &ast.OrPat{Alts: []ast.Pattern{first}}
	for p.got(token.Pipe) {
		or.Alts = append(or.Alts, p.parsePatAlt())
	}
	or.Sp = p.spanFrom(start)
	return or
}

func isLiteralKind(k token.Kind) bool {
	switch k {
	case token.Int, token.Float, token.Duration, token.Rune, token.String, token.RawString, token.Bytes:
		return true
	}
	return false
}

func (p *parser) parsePatAlt() ast.Pattern {
	start := p.tok.Span.Start
	t := p.tok
	switch {
	case t.Kind == token.Ident && t.Text == "_":
		p.next()
		return &ast.WildcardPat{Base: ast.Base{Sp: t.Span}}
	case t.Kind == token.Ident && (t.Text == "true" || t.Text == "false"):
		p.next()
		return &ast.LitPat{Base: ast.Base{Sp: t.Span}, Value: &ast.Ident{Base: ast.Base{Sp: t.Span}, Name: t.Text}}
	case t.Kind == token.Ident:
		path := []*ast.Ident{p.expectIdent("a name")}
		for p.at(token.Dot) && p.peek(1).Kind == token.Ident {
			p.next()
			path = append(path, p.expectIdent("a name"))
		}
		last := path[len(path)-1].Name
		switch {
		case p.at(token.LParen):
			v := &ast.VariantPat{Path: path, Parens: true}
			v.Fields, v.Rest = p.parsePatFields(token.LParen, token.RParen)
			v.Sp = p.spanFrom(start)
			return v
		case p.at(token.LBrace) && isUpper(last) && !p.noLit:
			// In a condition head (`if x is P {`) the brace opens the
			// block, as for struct literals (core 1.4).
			sp := &ast.StructPat{Path: path}
			sp.Fields, sp.Rest = p.parsePatFields(token.LBrace, token.RBrace)
			sp.Sp = p.spanFrom(start)
			return sp
		case len(path) == 1 && !isUpper(last):
			return &ast.BindPat{Base: ast.Base{Sp: t.Span}, Name: path[0]}
		}
		return &ast.VariantPat{Base: ast.Base{Sp: p.spanFrom(start)}, Path: path}
	case isLiteralKind(t.Kind) || (t.Kind == token.Minus && (p.peek(1).Kind == token.Int || p.peek(1).Kind == token.Float || p.peek(1).Kind == token.Duration)):
		lo := p.parsePatLiteral()
		if p.at(token.DotDot) || p.at(token.DotDotEq) {
			r := &ast.RangePat{Lo: lo, Inclusive: p.at(token.DotDotEq)}
			p.next()
			r.Hi = p.parsePatLiteral()
			r.Sp = p.spanFrom(start)
			return r
		}
		return &ast.LitPat{Base: ast.Base{Sp: p.spanFrom(start)}, Value: lo}
	case t.Kind == token.LParen:
		if p.peek(1).Kind == token.RParen {
			p.next()
			p.next()
			return &ast.TuplePat{Base: ast.Base{Sp: p.spanFrom(start)}}
		}
		var elems []ast.Pattern
		trailingComma := false
		sp := p.parseList(token.LParen, token.RParen, "tuple pattern", func() {
			elems = append(elems, p.parsePattern())
			trailingComma = p.at(token.Comma)
		})
		if len(elems) == 1 && !trailingComma {
			return elems[0]
		}
		return &ast.TuplePat{Base: ast.Base{Sp: sp}, Elems: elems}
	case t.Kind == token.LBrack:
		lp := &ast.ListPat{Rest: -1}
		lp.Sp = p.parseList(token.LBrack, token.RBrack, "list pattern", func() {
			if p.at(token.DotDot) {
				if lp.Rest >= 0 {
					p.errorf(diag.MisplacedRest, p.tok.Span, "a list pattern may contain `..` only once")
				}
				lp.Rest = len(lp.Elems)
				p.next()
				return
			}
			lp.Elems = append(lp.Elems, p.parsePattern())
		})
		return lp
	case t.Kind == token.DotDot:
		p.errorf(diag.MisplacedRest, t.Span, "`..` is only allowed inside list, variant, or struct patterns")
		p.next()
		return &ast.BadPat{Base: ast.Base{Sp: t.Span}}
	}
	p.errorf(diag.InvalidPattern, t.Span, "expected a pattern, found %s", describe(t))
	return &ast.BadPat{Base: ast.Base{Sp: token.Span{File: p.file, Start: t.Span.Start, End: t.Span.Start}}}
}

func (p *parser) parsePatLiteral() ast.Expr {
	start := p.tok.Span.Start
	if p.at(token.Minus) {
		p.next()
		x := p.parsePatLiteral()
		return &ast.UnaryExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Op: token.Minus, X: x}
	}
	if !isLiteralKind(p.tok.Kind) {
		p.errorf(diag.InvalidPattern, p.tok.Span, "expected a literal in a range pattern, found %s", describe(p.tok))
		return &ast.BadExpr{Base: ast.Base{Sp: p.tok.Span}}
	}
	return p.parsePrimary()
}

// parsePatFields parses `(fields)` or `{fields}` of a variant or struct
// pattern. A trailing `..` ignores the remaining fields.
func (p *parser) parsePatFields(open, close token.Kind) ([]*ast.PatField, bool) {
	var fields []*ast.PatField
	rest := false
	p.parseList(open, close, "pattern field", func() {
		if rest {
			p.errorf(diag.MisplacedRest, p.tok.Span, "`..` must be the last item in a pattern")
			return
		}
		if p.at(token.DotDot) {
			rest = true
			p.next()
			return
		}
		start := p.tok.Span.Start
		f := &ast.PatField{}
		if p.at(token.Ident) && p.peek(1).Kind == token.Colon {
			f.Name = p.expectIdent("a field name")
			p.next()
		}
		f.Pattern = p.parsePattern()
		f.Sp = p.spanFrom(start)
		fields = append(fields, f)
	})
	if fields == nil {
		fields = []*ast.PatField{}
	}
	return fields, rest
}
