package parser

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Statement habits (decision 0087).

// foreignStmt recognizes a statement written in another language at the
// current token. ok is false when none applies.
func (p *parser) foreignStmt() (ast.Stmt, bool) {
	t := p.tok
	start := t.Span.Start
	next := p.peek(1)
	switch t.Kind {
	case token.Let:
		if next.Kind == token.Ident && next.Text == "mut" && p.peek(2).Kind == token.Ident {
			d := p.foreign(t.Span.To(next.Span), "`let mut` is Rust syntax; Xo declares a mutable binding with `var` (core 3.1)")
			addFix(d, "write `var`", p.replaceSpan(t.Span.To(next.Span), "var"))
			p.next()
			p.next()
			return p.finishVar(start), true
		}
		return nil, false
	case token.Var:
		if next.Kind == token.Ident && p.peek(2).Span.Start.Line == next.Span.Start.Line &&
			(p.peek(2).Kind == token.Ident || p.peek(2).Kind == token.Star || p.peek(2).Kind == token.LBrack) {
			// Go: `var x int = 1`.
			p.next()
			s := &ast.VarStmt{Name: p.expectIdent("a variable name")}
			ty := p.tok
			if p.peek(1).Kind == token.Newline || p.peek(1).Kind == token.RBrace || p.peek(1).Kind == token.EOF {
				p.foreign(ty.Span, "`var %s %s` with no value is Go's zero value declaration; Xo has no zero values: give the binding its first value, `var %s: %s = ...` (core 3.1)", s.Name.Name, ty.Text, s.Name.Name, ty.Text)
			} else {
				d := p.foreign(ty.Span, "`var %s %s` is Go syntax; Xo writes the type after a colon, `var %s: %s` (core 3.1)", s.Name.Name, ty.Text, s.Name.Name, ty.Text)
				addFix(d, "insert `:`", p.replaceRange(s.Name.Sp.End, ty.Span.Start, ": "))
			}
			s.Type = p.parseType()
			if p.got(token.Assign) {
				s.Value = p.parseExpr()
			} else {
				s.Value = &ast.BadExpr{Base: ast.Base{Sp: s.Type.Span()}}
			}
			s.Sp = p.spanFrom(start)
			return s, true
		}
		return nil, false
	case token.Ident:
	default:
		return nil, false
	}
	if s, ok := p.shortDecl(); ok {
		return s, true
	}
	sameLine := next.Span.Start.Line == t.Span.Start.Line && next.Kind != token.Newline
	switch t.Text {
	case "val":
		if next.Kind == token.Ident && (p.peek(2).Kind == token.Assign || p.peek(2).Kind == token.Colon) {
			d := p.foreign(t.Span, "`val` is Kotlin syntax; Xo declares an immutable binding with `let` (core 3.1)")
			addFix(d, "write `let`", p.replaceSpan(t.Span, "let"))
			p.next()
			s := &ast.LetStmt{Pattern: &ast.BindPat{Base: ast.Base{Sp: p.tok.Span}, Name: p.expectIdent("a name")}}
			if p.got(token.Colon) {
				s.Type = p.parseType()
			}
			p.expect(token.Assign, "`=` in `let`")
			s.Value = p.parseExpr()
			s.Sp = p.spanFrom(start)
			return s, true
		}
	case "throw", "raise":
		if !sameLine || !startsExpr(next.Kind) {
			return nil, false
		}
		lang := "TypeScript, Kotlin, and Swift"
		if t.Text == "raise" {
			lang = "Python"
		}
		p.next()
		if p.atIdent("new") && p.peek(1).Kind == token.Ident {
			p.next()
		}
		x := p.parseExpr()
		d := p.foreign(t.Span, "`%s` is %s syntax; Xo has no exceptions: a function declared `-> T ! E` fails by returning `Err(e)`, and callers propagate with `?` (core 2.6)", t.Text, lang)
		addFix(d, "write `return Err(...)`", p.replaceRange(t.Span.Start, x.Span().Start, "return Err("), p.insertAt(x.Span().End, ")"))
		call := &ast.CallExpr{Base: ast.Base{Sp: x.Span()}, Fun: ident("Err", t.Span), Args: []*ast.Arg{{Base: ast.Base{Sp: x.Span()}, Value: x}}}
		return &ast.ReturnStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Value: call}, true
	case "go":
		if !sameLine || (next.Kind != token.Ident && next.Kind != token.Fn) {
			return nil, false
		}
		p.next()
		x := p.parseExpr()
		p.foreign(t.Span, "`go f()` is Go syntax; Xo starts a task inside a scope so it cannot outlive its caller: `scope s { s.spawn(fn() { f() }) }` (core 7.1)")
		return &ast.ExprStmt{Base: ast.Base{Sp: p.spanFrom(start)}, X: x}, true
	case "guard":
		if next.Kind != token.Let {
			return nil, false
		}
		return p.guardLet(), true
	case "loop":
		if next.Kind != token.LBrace {
			return nil, false
		}
		d := p.foreign(t.Span, "`loop` is Rust syntax; Xo writes an infinite loop as `while { ... }` (core 3.5)")
		addFix(d, "write `while`", p.replaceSpan(t.Span, "while"))
		return p.parseWhile(nil, start), true
	}
	return nil, false
}

// finishVar parses the rest of `var name [: T] = value` after `var`.
func (p *parser) finishVar(start token.Pos) ast.Stmt {
	s := &ast.VarStmt{Name: p.expectIdent("a variable name")}
	if p.got(token.Colon) {
		s.Type = p.parseType()
	}
	p.expect(token.Assign, "`=` in `var`")
	s.Value = p.parseExpr()
	s.Sp = p.spanFrom(start)
	return s
}

// shortDecl recognizes Go's `x := v`, `a, b := f()`, and the error idiom
// `x, err := f()` followed by `if err != nil { return ... }`.
func (p *parser) shortDecl() (ast.Stmt, bool) {
	i := p.pos
	var names []token.Token
	for {
		if p.toks[i].Kind != token.Ident {
			return nil, false
		}
		names = append(names, p.toks[i])
		i++
		if p.toks[i].Kind == token.Comma {
			i++
			continue
		}
		break
	}
	colon, assign := p.toks[i], p.toks[i+1]
	if colon.Kind != token.Colon || assign.Kind != token.Assign || !adjacent(colon, assign) {
		return nil, false
	}
	start := names[0].Span.Start
	p.skipTo(i + 2)
	value := p.parseExpr()
	head := names[0].Span.To(assign.Span)
	bind := func(n token.Token) ast.Pattern {
		if n.Text == "_" {
			return &ast.WildcardPat{Base: ast.Base{Sp: n.Span}}
		}
		return &ast.BindPat{Base: ast.Base{Sp: n.Span}, Name: ident(n.Text, n.Span)}
	}
	if len(names) == 1 {
		d := p.foreign(head, "`%s := v` is Go syntax; Xo binds with `let %s = v`, or `var %s = v` when it is reassigned later (core 3.1)", names[0].Text, names[0].Text, names[0].Text)
		if p.assignedLater(names[0].Text) {
			addFix(d, "write `var`", p.replaceSpan(head, "var "+names[0].Text+" ="))
			return &ast.VarStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Name: ident(names[0].Text, names[0].Span), Value: value}, true
		}
		addFix(d, "write `let`", p.replaceSpan(head, "let "+names[0].Text+" ="))
		return &ast.LetStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Pattern: bind(names[0]), Value: value}, true
	}
	last := names[len(names)-1]
	if len(names) == 2 && (last.Text == "err" || last.Text == "e") {
		d := p.foreign(head, "`%s, %s := f()` is Go's error return; an Xo function that can fail returns a Result: `let %s = f()?` propagates the error to the caller, and `match f() { Ok(v) => ... Err(e) => ... }` handles it (core 2.6)", names[0].Text, last.Text, names[0].Text)
		try := &ast.TryExpr{Base: ast.Base{Sp: value.Span()}, X: value}
		var res ast.Stmt = &ast.LetStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Pattern: bind(names[0]), Value: try}
		if names[0].Text == "_" {
			res = &ast.ExprStmt{Base: ast.Base{Sp: p.spanFrom(start)}, X: try}
		}
		// The idiom: the next statement is `if err != nil { return ... }`.
		if blockEnd, ok := p.errCheckAhead(last.Text); ok {
			repl := "let " + names[0].Text + " = "
			if names[0].Text == "_" {
				repl = ""
			}
			ifTok := p.peekPastNewlines()
			edits := []diag.Edit{
				p.replaceRange(names[0].Span.Start, p.afterBlanks(assign.Span.End), repl),
				p.insertAt(value.Span().End, "?"),
				p.replaceRange(p.lineStart(ifTok.Span.Start), p.nextLineStart(p.toks[blockEnd].Span.End), ""),
			}
			addFix(d, "propagate with `?`", edits...)
			// Consume the error check: it has no Xo meaning.
			p.skipNewlines()
			p.skipTo(blockEnd + 1)
		}
		return res, true
	}
	d := p.foreign(head, "`a, b := f()` is Go syntax; Xo destructures a tuple with `let (a, b) = f()` (core 3.1, 3.5)")
	var texts []string
	var pats []ast.Pattern
	for _, n := range names {
		texts = append(texts, n.Text)
		pats = append(pats, bind(n))
	}
	addFix(d, "write `let (...) =`", p.replaceSpan(head, "let ("+strings.Join(texts, ", ")+") ="))
	return &ast.LetStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Pattern: &ast.TuplePat{Base: ast.Base{Sp: head}, Elems: pats}, Value: value}, true
}

// assignedLater reports whether name is assigned or mutated later in the
// enclosing block: `name =`, `name += ...`, `name.f = ...`, `name++`, or a
// mutating method call `name.push(...)` at the start of a statement.
func (p *parser) assignedLater(name string) bool {
	depth := 0
	stmtStart := true
	for i := p.pos; i < len(p.toks); i++ {
		t := p.toks[i]
		switch t.Kind {
		case token.EOF:
			return false
		case token.LBrace, token.LParen, token.LBrack, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack:
			depth--
		case token.RBrace:
			depth--
			if depth < 0 {
				return false
			}
		}
		if t.Kind == token.Newline || t.Kind == token.LBrace {
			stmtStart = true
			continue
		}
		if stmtStart && t.Kind == token.Ident && t.Text == name {
			j := i + 1
			for j+1 < len(p.toks) && p.toks[j].Kind == token.Dot && p.toks[j+1].Kind == token.Ident {
				if k := p.toks[j+2]; k.Kind == token.LParen && mutatingMethods[p.toks[j+1].Text] {
					return true
				}
				j += 2
			}
			k := p.toks[j]
			if k.Kind.IsAssign() || ((k.Kind == token.Plus || k.Kind == token.Minus) && p.toks[j+1].Kind == k.Kind) {
				return true
			}
		}
		stmtStart = false
	}
	return false
}

// mutatingMethods are the std methods that need a `var` receiver.
var mutatingMethods = map[string]bool{"push": true, "pop": true, "put": true, "remove": true, "insert": true, "set": true, "add": true, "update": true, "remove_at": true}

// errCheckAhead reports whether the next statement is Go's
// `if err != nil { return ... }` with a single return in its block, and
// returns the index of its closing brace.
func (p *parser) errCheckAhead(name string) (int, bool) {
	i := p.pos
	for i < len(p.toks) && p.toks[i].Kind == token.Newline {
		i++
	}
	if i == p.pos {
		return 0, false
	}
	k := func(j int) token.Token { return p.toks[min(j, len(p.toks)-1)] }
	if k(i).Kind != token.If || k(i+1).Kind != token.Ident || k(i+1).Text != name ||
		k(i+2).Kind != token.NotEq || k(i+3).Kind != token.Ident || k(i+3).Text != "nil" || k(i+4).Kind != token.LBrace {
		return 0, false
	}
	end := p.matchClose(i + 4)
	if end < 0 {
		return 0, false
	}
	stmts, depth, inStmt := 0, 0, false
	for j := i + 5; j < end; j++ {
		tk := p.toks[j].Kind
		if depth == 0 && tk == token.Newline {
			inStmt = false
			continue
		}
		if !inStmt {
			stmts++
			inStmt = true
			if stmts == 1 && tk != token.Return {
				return 0, false
			}
		}
		switch tk {
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
		}
	}
	if stmts != 1 {
		return 0, false
	}
	if k(end+1).Kind != token.Newline && k(end+1).Kind != token.RBrace && k(end+1).Kind != token.EOF {
		return 0, false
	}
	return end, true
}

// guardLet rewrites Swift's `guard let v = x else { ... }` as
// `let v = if let Some(v) = x { v } else { ... }`.
func (p *parser) guardLet() ast.Stmt {
	g := p.tok
	start := g.Span.Start
	p.next() // guard
	p.next() // let
	name := p.expectIdent("a name")
	p.expect(token.Assign, "`=` in `guard let`")
	value := p.parseExprNoLit()
	elseTok := p.tok
	if !p.at(token.Else) {
		p.errorExpected("`else` in `guard let`")
		return &ast.BadStmt{Base: ast.Base{Sp: g.Span}}
	}
	p.next()
	body := p.parseBlock()
	d := p.foreign(g.Span, "`guard let` is Swift syntax; Xo writes `let %s = if let Some(%s) = x { %s } else { ... }`, or `let %s = x ?? default` (core 2.5)", name.Name, name.Name, name.Name, name.Name)
	addFix(d, "write `let ... = if let Some(...)`",
		p.replaceRange(g.Span.Start, name.Sp.Start, "let "),
		p.replaceRange(name.Sp.End, value.Span().Start, " = if let Some("+name.Name+") = "),
		p.replaceRange(value.Span().End, elseTok.Span.End, " { "+name.Name+" } else"))
	ifx := &ast.IfExpr{Base: ast.Base{Sp: p.spanFrom(start)},
		Pattern: &ast.VariantPat{Base: ast.Base{Sp: name.Sp}, Path: []*ast.Ident{ident("Some", name.Sp)}, Parens: true,
			Fields: []*ast.PatField{{Base: ast.Base{Sp: name.Sp}, Pattern: &ast.BindPat{Base: ast.Base{Sp: name.Sp}, Name: ident(name.Name, name.Sp)}}}},
		Value: value, Then: exprBlock(ident(name.Name, name.Sp)), Else: body}
	return &ast.LetStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Pattern: &ast.BindPat{Base: ast.Base{Sp: name.Sp}, Name: name}, Value: ifx}
}

// foreignPostStmt handles `x++` and `x--` after the expression x of a
// statement. ok is false when the current tokens are not that.
func (p *parser) foreignIncDec(x ast.Expr, start token.Pos) (ast.Stmt, bool) {
	t := p.tok
	if (t.Kind != token.Plus && t.Kind != token.Minus) || p.peek(1).Kind != t.Kind || !adjacent(t, p.peek(1)) {
		return nil, false
	}
	end := p.peek(1)
	op, repl := token.PlusAssign, " += 1"
	if t.Kind == token.Minus {
		op, repl = token.MinusAssign, " -= 1"
	}
	d := p.foreign(t.Span.To(end.Span), "`%s%s%s` is Go, TypeScript, Kotlin, and Swift syntax; Xo writes `%s%s` (core 3.5)", p.spanText(x.Span()), t.Kind, t.Kind, p.spanText(x.Span()), repl)
	addFix(d, "write `"+strings.TrimSpace(repl)+"`", p.replaceSpan(t.Span.To(end.Span), repl))
	p.next()
	p.next()
	one := &ast.IntLit{Base: ast.Base{Sp: end.Span}, Raw: "1", Value: "1"}
	return &ast.AssignStmt{Base: ast.Base{Sp: p.spanFrom(start)}, Op: op, Target: x, Value: one}, true
}

// foreignReturnList handles Go's `return a, b` after the first value.
func (p *parser) foreignReturnList(first ast.Expr, retTok token.Token) ast.Expr {
	vals := []ast.Expr{first}
	for p.got(token.Comma) {
		vals = append(vals, p.parseExpr())
	}
	sp := first.Span().To(vals[len(vals)-1].Span())
	isNil := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && id.Name == "nil"
	}
	zero := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name == "nil" || x.Name == "false"
		case *ast.IntLit:
			return x.Value == "0"
		case *ast.StringLit:
			return len(x.Parts) == 0 || (len(x.Parts) == 1 && x.Parts[0].Expr == nil && x.Parts[0].Text == "")
		}
		return false
	}
	last := vals[len(vals)-1]
	switch {
	case len(vals) == 2 && isNil(last):
		d := p.foreign(sp, "`return v, nil` is Go's success return; an Xo function declared `-> T ! E` returns just the value: `return %s` (core 2.6)", p.spanText(first.Span()))
		addFix(d, "return the value", p.replaceRange(first.Span().End, last.Span().End, ""))
		return first
	case len(vals) == 2 && zero(first):
		d := p.foreign(sp, "`return zero, err` is Go's failure return; an Xo function declared `-> T ! E` fails with `return Err(%s)` (core 2.6)", p.spanText(last.Span()))
		addFix(d, "write `Err(...)`", p.replaceRange(first.Span().Start, last.Span().Start, "Err("), p.insertAt(last.Span().End, ")"))
		return &ast.CallExpr{Base: ast.Base{Sp: sp}, Fun: ident("Err", first.Span()), Args: []*ast.Arg{{Base: ast.Base{Sp: last.Span()}, Value: last}}}
	}
	d := p.foreign(sp, "`return a, b` is Go syntax for several results; Xo returns one value, here a tuple: `return (a, b)` (core 2.1)")
	addFix(d, "return a tuple", p.insertAt(first.Span().Start, "("), p.insertAt(sp.End, ")"))
	_ = retTok
	return &ast.TupleExpr{Base: ast.Base{Sp: sp}, Elems: vals}
}

// forHeader describes the tokens between `for` and the `{` of the body on
// the same line.
type forHeader struct {
	braceIdx  int  // index of the `{` that opens the body, or -1
	hasIn     bool // `in` at depth zero
	hasSemi   bool // `;` at depth zero (C style)
	shortDecl bool // `:=` at depth zero
	rangeKw   bool // Go's `range` after `:=`
	notExpr   bool // two operands in a row at depth zero: not one expression
}

func (p *parser) scanForHeader() forHeader {
	h := forHeader{braceIdx: -1}
	depth := 0
	prevOperand := false
	for i := p.pos + 1; i < len(p.toks); i++ {
		t := p.toks[i]
		operand := false
		switch t.Kind {
		case token.Ident, token.Int, token.Float, token.String, token.RawString, token.Duration, token.Rune:
			operand = true
		}
		if depth == 0 && operand && prevOperand {
			h.notExpr = true
		}
		if depth == 0 {
			prevOperand = operand || t.Kind == token.RParen || t.Kind == token.RBrack
		}
		switch t.Kind {
		case token.EOF:
			return h
		case token.Newline:
			if t.Text != ";" {
				return h
			}
			if depth == 0 {
				h.hasSemi = true
			}
		case token.LParen, token.LBrack, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
		case token.LBrace:
			// A brace after a type name is a struct pattern, not the body.
			if prev := p.toks[i-1]; depth == 0 && !(prev.Kind == token.Ident && isUpper(prev.Text)) {
				h.braceIdx = i
				return h
			}
			depth++
		case token.In:
			if depth == 0 {
				h.hasIn = true
			}
		case token.Colon:
			if depth == 0 && p.toks[i+1].Kind == token.Assign && adjacent(t, p.toks[i+1]) {
				h.shortDecl = true
				if k := p.toks[i+2]; k.Kind == token.Ident && k.Text == "range" {
					h.rangeKw = true
				}
			}
		}
	}
	return h
}

// foreignFor recognizes loops written in another language at `for`.
func (p *parser) foreignFor(label *ast.Ident, start token.Pos) (ast.Stmt, bool) {
	forTok := p.tok
	if p.peek(1).Kind == token.LBrace {
		d := p.foreign(forTok.Span, "`for { ... }` is Go's infinite loop; Xo writes `while { ... }` (core 3.5)")
		addFix(d, "write `while`", p.replaceSpan(forTok.Span, "while"))
		return p.parseWhile(label, start), true
	}
	if p.peek(1).Kind == token.LParen {
		if s, ok := p.parenFor(label, start); ok {
			return s, true
		}
		return nil, false
	}
	h := p.scanForHeader()
	switch {
	case h.braceIdx < 0:
		return nil, false
	case h.rangeKw:
		return p.goRangeFor(label, start), true
	case h.shortDecl && h.hasSemi:
		return p.cFor(label, start, false), true
	case !h.hasIn && !h.shortDecl && !h.hasSemi && !h.notExpr:
		d := p.foreign(forTok.Span, "`for cond { ... }` is Go's while loop; Xo writes `while cond { ... }` (core 3.5)")
		addFix(d, "write `while`", p.replaceSpan(forTok.Span, "while"))
		return p.parseWhile(label, start), true
	}
	return nil, false
}

// parenFor handles `for (x in xs)`, `for (const x of xs)`, and
// `for (let i = 0; i < n; i++)`.
func (p *parser) parenFor(label *ast.Ident, start token.Pos) (ast.Stmt, bool) {
	open := p.pos + 1
	close := p.matchClose(open)
	if close < 0 || p.toks[close+1].Kind != token.LBrace {
		return nil, false
	}
	semi, in, of := false, -1, -1
	depth := 0
	for i := open + 1; i < close; i++ {
		t := p.toks[i]
		switch t.Kind {
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
		case token.Newline:
			if t.Text == ";" && depth == 0 {
				semi = true
			}
		case token.In:
			if depth == 0 && in < 0 {
				in = i
			}
		case token.Ident:
			if depth == 0 && t.Text == "of" && of < 0 {
				of = i
			}
		}
	}
	if semi {
		return p.cFor(label, start, true), true
	}
	kw := in
	if kw < 0 {
		kw = of
	}
	if kw < 0 {
		return nil, false
	}
	forTok := p.tok
	p.next() // for
	lp := p.tok
	p.next() // (
	declTok := token.Token{}
	if (p.at(token.Let) || p.at(token.Var) || p.at(token.Const) || p.atIdent("val")) && p.peek(1).Kind == token.Ident {
		declTok = p.tok
		p.next()
	}
	s := &ast.ForStmt{Label: label, Pattern: p.parsePattern()}
	kwTok := p.tok
	p.next() // in / of
	s.Iter = p.parseExpr()
	rp := p.tok
	p.expect(token.RParen, "`)`")
	d := p.foreign(lp.Span, "`for (x in xs)` is TypeScript, Kotlin, and Java syntax; Xo writes `for x in xs` with no parentheses (core 3.5)")
	edits := []diag.Edit{p.replaceRange(lp.Span.Start, p.afterBlanks(lp.Span.End), "")}
	if declTok.Kind != token.Illegal || declTok.Text != "" {
		if declTok.Text != "" {
			edits = append(edits, p.deleteToken(declTok))
		}
	}
	if kwTok.Kind != token.In {
		edits = append(edits, p.replaceSpan(kwTok.Span, "in"))
	}
	edits = append(edits, p.replaceSpan(rp.Span, ""))
	addFix(d, "remove the parentheses", edits...)
	_ = forTok
	s.Body = p.parseCondBlock(s.Iter)
	s.Sp = p.spanFrom(start)
	return s, true
}

// cFor parses a C style counting loop: Go's `for i := 0; i < n; i++ {`
// or, with paren set, `for (let i = 0; i < n; i++) {`.
func (p *parser) cFor(label *ast.Ident, start token.Pos, paren bool) ast.Stmt {
	forTok := p.tok
	p.next() // for
	if paren {
		p.next() // (
		if p.at(token.Let) || p.at(token.Var) || p.at(token.Const) || (p.at(token.Ident) && p.peek(1).Kind == token.Ident) {
			p.next() // let, var, or a C type such as int
		}
	}
	name := p.expectIdent("a loop variable")
	if p.at(token.Colon) {
		p.next()
	}
	p.expect(token.Assign, "`=` in the loop initializer")
	lo := p.parseExpr()
	p.expect(token.Newline, "`;` after the loop initializer")
	var cond ast.Expr
	if !p.at(token.Newline) {
		cond = p.parseExprNoLit()
	}
	p.expect(token.Newline, "`;` after the loop condition")
	// The post statement, as text.
	closeKind := token.LBrace
	if paren {
		closeKind = token.RParen
	}
	var post strings.Builder
	for !p.at(closeKind) && !p.at(token.EOF) && !p.at(token.Newline) {
		post.WriteString(p.tok.Text)
		p.next()
	}
	if paren {
		p.expect(token.RParen, "`)`")
	}
	headerEnd := p.tok.Span.Start
	n := name.Name
	step := post.String()
	up := step == n+"++" || step == "++"+n || step == n+"+=1" || step == n+"="+n+"+1"
	var hi ast.Expr
	inclusive := false
	if be, ok := cond.(*ast.BinaryExpr); ok && up {
		if id, ok := be.X.(*ast.Ident); ok && id.Name == n && (be.Op == token.Lt || be.Op == token.LtEq) {
			hi = be.Y
			inclusive = be.Op == token.LtEq
		}
	}
	d := p.foreign(forTok.Span, "a C style `for init; cond; step` loop is Go, TypeScript, and Java syntax; Xo counts with a range, `for %s in a..b` (`a..=b` includes `b`), or loops with `while cond` (core 3.5)", n)
	r := &ast.RangeExpr{Base: ast.Base{Sp: lo.Span()}, Lo: lo, Hi: lo}
	if hi != nil {
		r.Hi = hi
		r.Inclusive = inclusive
		r.Sp = lo.Span().To(hi.Span())
		dots := ".."
		if inclusive {
			dots = "..="
		}
		addFix(d, "write `for "+n+" in ...`", p.replaceRange(forTok.Span.Start, headerEnd, "for "+n+" in "+p.spanText(lo.Span())+dots+p.spanText(hi.Span())+" "))
	} else {
		addNote(d, "a loop that counts down or steps by more than one is `var i = a` with `while i > b { ...; i -= 1 }`")
	}
	s := &ast.ForStmt{Label: label, Pattern: &ast.BindPat{Base: ast.Base{Sp: name.Sp}, Name: name}, Iter: r}
	s.Body = p.parseBlock()
	s.Sp = p.spanFrom(start)
	return s
}

// goRangeFor parses Go's `for k, v := range x {`. The checker reports it,
// because the Xo form depends on the type of x.
func (p *parser) goRangeFor(label *ast.Ident, start token.Pos) ast.Stmt {
	p.next() // for
	var pats []ast.Pattern
	for p.at(token.Ident) {
		t := p.tok
		if t.Text == "_" {
			pats = append(pats, &ast.WildcardPat{Base: ast.Base{Sp: t.Span}})
		} else {
			pats = append(pats, &ast.BindPat{Base: ast.Base{Sp: t.Span}, Name: ident(t.Text, t.Span)})
		}
		p.next()
		if !p.got(token.Comma) {
			break
		}
	}
	p.next() // :
	p.next() // =
	p.next() // range
	s := &ast.ForStmt{Label: label, GoRange: true}
	if len(pats) == 1 {
		s.Pattern = pats[0]
	} else {
		s.Pattern = &ast.TuplePat{Base: ast.Base{Sp: pats[0].Span().To(pats[len(pats)-1].Span())}, Elems: pats}
	}
	s.Iter = p.parseExprNoLit()
	s.Body = p.parseCondBlock(s.Iter)
	s.Sp = p.spanFrom(start)
	return s
}
