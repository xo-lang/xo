package parser

import (
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Expression habits (decision 0087).

// incDecAhead reports whether the current tokens are `++` or `--` ending a
// statement, which the binary operator loop must leave alone.
func (p *parser) incDecAhead() bool {
	t := p.tok
	if (t.Kind != token.Plus && t.Kind != token.Minus) || p.peek(1).Kind != t.Kind || !adjacent(t, p.peek(1)) {
		return false
	}
	switch p.peek(2).Kind {
	case token.Newline, token.RBrace, token.EOF:
		return true
	}
	return false
}

// foreignBinary handles binary operators from other languages at the
// current token, after the left operand x. ok is false when none applies.
func (p *parser) foreignBinary(x ast.Expr, minPrec int) (ast.Expr, bool) {
	if p.bad {
		return nil, false
	}
	t := p.tok
	next := p.peek(1)
	switch {
	case t.Kind == token.Ident && (t.Text == "and" || t.Text == "or") && startsExpr(next.Kind):
		op, prec, sym := token.AndAnd, 3, "&&"
		if t.Text == "or" {
			op, prec, sym = token.OrOr, 2, "||"
		}
		if prec < minPrec {
			return nil, false
		}
		d := p.foreign(t.Span, "`%s` is Python syntax; Xo writes `%s` (core 15 operators)", t.Text, sym)
		addFix(d, "write `"+sym+"`", p.replaceSpan(t.Span, sym))
		p.next()
		y := p.parseBinary(prec + 1)
		return &ast.BinaryExpr{Base: ast.Base{Sp: x.Span().To(y.Span())}, Op: op, X: x, Y: y}, true
	case t.Kind == token.Ident && t.Text == "not" && next.Kind == token.In:
		if isPrec < minPrec {
			return nil, false
		}
		p.next()
		p.next()
		y := p.parseBinary(isPrec + 1)
		d := p.foreign(t.Span.To(next.Span), "`x not in xs` is Python syntax; Xo writes `!xs.contains(x)` (spec/std.md 2)")
		addFix(d, "write `!xs.contains(x)`", p.replaceRange(x.Span().Start, y.Span().End, "!"+p.operandText(y)+".contains("+p.spanText(x.Span())+")"))
		sp := x.Span().To(y.Span())
		return &ast.UnaryExpr{Base: ast.Base{Sp: sp}, Op: token.Bang, X: methodCall(y, "contains", sp, x)}, true
	case t.Kind == token.In:
		if isPrec < minPrec {
			return nil, false
		}
		p.next()
		y := p.parseBinary(isPrec + 1)
		d := p.foreign(t.Span, "`x in xs` as a test is Python, TypeScript, and Kotlin syntax; Xo writes `xs.contains(x)`, and `in` appears only in `for` (spec/std.md 2)")
		addFix(d, "write `xs.contains(x)`", p.replaceRange(x.Span().Start, y.Span().End, p.operandText(y)+".contains("+p.spanText(x.Span())+")"))
		sp := x.Span().To(y.Span())
		return methodCall(y, "contains", sp, x), true
	case (t.Kind == token.Eq || t.Kind == token.NotEq) && next.Kind == token.Assign && adjacent(t, next):
		if 4 < minPrec {
			return nil, false
		}
		d := p.foreign(t.Span.To(next.Span), "`%s=` is TypeScript strict equality; Xo has one equality, `%s`, and it never converts types (core 2.10)", t.Kind, t.Kind)
		addFix(d, "write `"+t.Kind.String()+"`", p.replaceSpan(next.Span, ""))
		p.next()
		p.next()
		y := p.parseBinary(5)
		return &ast.BinaryExpr{Base: ast.Base{Sp: x.Span().To(y.Span())}, Op: t.Kind, X: x, Y: y}, true
	case t.Kind == token.Star && next.Kind == token.Star && adjacent(t, next):
		if 7 < minPrec {
			return nil, false
		}
		p.next()
		p.next()
		y := p.parseUnary()
		d := p.foreign(t.Span.To(next.Span), "`**` is Python, TypeScript, and Kotlin syntax for a power; Xo writes `x.pow(n)` (spec/std.md 2)")
		addFix(d, "write `.pow(...)`", p.replaceRange(x.Span().Start, y.Span().End, p.operandText(x)+".pow("+p.spanText(y.Span())+")"))
		return methodCall(x, "pow", x.Span().To(y.Span()), y), true
	case t.Kind == token.Question && next.Kind == token.Colon && adjacent(t, next):
		if 1 < minPrec {
			return nil, false
		}
		d := p.foreign(t.Span.To(next.Span), "`?:` is Kotlin's elvis operator; Xo writes `??` (core 2.5)")
		addFix(d, "write `??`", p.replaceSpan(t.Span.To(next.Span), "??"))
		p.next()
		p.next()
		y := p.parseBinary(1)
		return &ast.BinaryExpr{Base: ast.Base{Sp: x.Span().To(y.Span())}, Op: token.Coalesce, X: x, Y: y}, true
	case t.Kind == token.Ident && t.Text == "until" && startsExpr(next.Kind):
		if rangePrec < minPrec {
			return nil, false
		}
		d := p.foreign(t.Span, "`a until b` is Kotlin syntax; Xo's `a..b` already excludes `b` (core 3.5)")
		addFix(d, "write `..`", p.replaceRange(p.beforeBlanks(t.Span.Start), p.afterBlanks(t.Span.End), ".."))
		p.next()
		y := p.parseBinary(rangePrec + 1)
		return &ast.RangeExpr{Base: ast.Base{Sp: x.Span().To(y.Span())}, Lo: x, Hi: y}, true
	}
	return nil, false
}

// foreignRange handles Swift's `..<` and `...` after `..`. It reports
// whether the range is inclusive.
func (p *parser) foreignRange(dots token.Token) bool {
	t := p.tok
	if t.Kind == token.Lt {
		d := p.foreign(dots.Span.To(t.Span), "`..<` is Swift and Kotlin syntax; Xo's `a..b` already excludes `b` (core 3.5)")
		addFix(d, "write `..`", p.replaceSpan(t.Span, ""))
		p.next()
		return false
	}
	d := p.foreign(dots.Span.To(t.Span), "`a...b` is Swift's closed range; Xo writes `a..=b` (core 3.5)")
	addFix(d, "write `..=`", p.replaceSpan(dots.Span.To(t.Span), "..="))
	p.next()
	return true
}

// ternaryAhead reports whether the `?` at the current token starts the
// `c ? a : b` conditional of C family languages rather than Xo's postfix
// `?`.
func (p *parser) ternaryAhead() bool {
	n := p.peek(1)
	switch n.Kind {
	case token.Ident:
		if n.Text == "is" || n.Text == "with" {
			return false
		}
	case token.Int, token.Float, token.Duration, token.Rune, token.String, token.RawString, token.Bytes, token.Bang, token.Hole:
	case token.LParen, token.LBrack, token.Minus:
		if adjacent(p.tok, n) {
			return false
		}
	default:
		return false
	}
	depth := 0
	for i := p.pos + 1; i < len(p.toks); i++ {
		switch p.tokAt(i).Kind {
		case token.Newline, token.EOF:
			return false
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
			if depth < 0 {
				return false
			}
		case token.Comma, token.FatArrow, token.Assign:
			// `{"a": x? - 1, "b": 2}`: the colon belongs to the next entry.
			if depth == 0 {
				return false
			}
		case token.Colon:
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// ternary parses `? a : b` after the condition c.
func (p *parser) ternary(c ast.Expr) ast.Expr {
	q := p.tok
	p.next() // ?
	a := p.parseBinary(1)
	colon := p.tok
	if !p.at(token.Colon) {
		p.errorExpected("`:` in a conditional expression")
		return c
	}
	p.next()
	b := p.parseExpr()
	d := p.foreign(q.Span, "`c ? a : b` is C, Java, TypeScript, and Swift syntax; Xo writes `if c { a } else { b }`, which is an expression (core 3.5)")
	addFix(d, "write `if ... else`",
		p.insertAt(c.Span().Start, "if "),
		p.replaceRange(c.Span().End, a.Span().Start, " { "),
		p.replaceRange(a.Span().End, b.Span().Start, " } else { "),
		p.insertAt(b.Span().End, " }"))
	_ = colon
	return &ast.IfExpr{Base: ast.Base{Sp: c.Span().To(b.Span())}, Cond: c, Then: exprBlock(a), Else: exprBlock(b)}
}

// pyCondAhead reports whether `if` after an expression starts Python's
// `a if c else b`.
func (p *parser) pyCondAhead() bool {
	if !p.at(token.If) || p.noLit {
		return false
	}
	depth := 0
	for i := p.pos + 1; i < len(p.toks); i++ {
		switch p.tokAt(i).Kind {
		case token.Newline, token.EOF, token.LBrace:
			if p.tokAt(i).Kind == token.LBrace && depth == 0 {
				return false
			}
			if p.tokAt(i).Kind != token.LBrace {
				return false
			}
			depth++
		case token.LParen, token.LBrack, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
			if depth < 0 {
				return false
			}
		case token.Else:
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// pyCond parses ` if c else b` after a.
func (p *parser) pyCond(a ast.Expr) ast.Expr {
	ifTok := p.tok
	p.next()
	c := p.parseBinary(2)
	elseTok := p.tok
	p.next() // else
	b := p.parseExpr()
	d := p.foreign(ifTok.Span, "`a if c else b` is Python syntax; Xo writes `if c { a } else { b }`, which is an expression (core 3.5)")
	addFix(d, "write `if ... else`",
		p.replaceRange(a.Span().Start, c.Span().End, "if "+p.spanText(c.Span())+" { "+p.spanText(a.Span())+" }"),
		p.replaceRange(elseTok.Span.End, b.Span().Start, " { "),
		p.insertAt(b.Span().End, " }"))
	return &ast.IfExpr{Base: ast.Base{Sp: a.Span().To(b.Span())}, Cond: c, Then: exprBlock(a), Else: exprBlock(b)}
}

// foreignUnary handles prefix operators from other languages: `&x`,
// `&mut x`, `*p`, `not x`, `await x`. ok is false when none applies.
func (p *parser) foreignUnary() (ast.Expr, bool) {
	t := p.tok
	next := p.peek(1)
	switch {
	case t.Kind == token.Amp && startsExpr(next.Kind):
		end := next
		if next.Kind == token.Ident && next.Text == "mut" && startsExpr(p.peek(2).Kind) {
			end = p.peek(2)
		}
		d := p.foreign(t.Span, "`%s` takes a pointer or reference in Go, Rust, and Swift; Xo has none: pass the value itself, and a `var` parameter changes the caller's `var` binding (core 3.2, 5.1)", strings.TrimSpace(p.srcText(t.Span.Start, end.Span.Start))+"x")
		addFix(d, "remove `"+strings.TrimSpace(p.srcText(t.Span.Start, end.Span.Start))+"`", p.replaceRange(t.Span.Start, end.Span.Start, ""))
		for p.tok.Span.Start.Offset < end.Span.Start.Offset {
			p.next()
		}
		return p.parseUnary(), true
	case t.Kind == token.Star && startsExpr(next.Kind) && adjacent(t, next):
		d := p.foreign(t.Span, "`*p` dereferences a pointer in Go and Rust; Xo has no pointers: use the value `p` (core 5.1)")
		addFix(d, "remove `*`", p.replaceSpan(t.Span, ""))
		p.next()
		return p.parseUnary(), true
	case t.Kind == token.Ident && t.Text == "not" && next.Kind != token.Minus && next.Kind != token.LBrace && startsExpr(next.Kind) && next.Kind != token.LParen || (t.Kind == token.Ident && t.Text == "not" && next.Kind == token.LParen && !adjacent(t, next)):
		d := p.foreign(t.Span, "`not` is Python syntax; Xo writes `!` (core 15 operators)")
		addFix(d, "write `!`", p.replaceRange(t.Span.Start, next.Span.Start, "!"))
		p.next()
		x := p.parseUnary()
		return &ast.UnaryExpr{Base: ast.Base{Sp: p.spanFrom(t.Span.Start)}, Op: token.Bang, X: x}, true
	case t.Kind == token.Ident && t.Text == "await" && startsExpr(next.Kind) && next.Kind != token.Minus && next.Kind != token.LBrace && next.Span.Start.Line == t.Span.Start.Line:
		d := p.foreign(t.Span, "`await` is TypeScript, Python, Rust, and Swift syntax; an Xo call blocks the calling task until it returns, so there is nothing to await; run work concurrently with `scope s { s.spawn(...) }` (core 7.1)")
		addFix(d, "remove `await`", p.deleteToken(t))
		p.next()
		return p.parseUnary(), true
	}
	return nil, false
}

// foreignPrimary handles operands written in another language's syntax.
// ok is false when none applies.
func (p *parser) foreignPrimary() (ast.Expr, bool) {
	t := p.tok
	next := p.peek(1)
	sameLine := next.Span.Start.Line == t.Span.Start.Line && next.Kind != token.Newline
	switch t.Kind {
	case token.Rune:
		if t.Value == "" && len(t.Text) >= 4 && strings.HasSuffix(t.Text, "'") {
			return p.singleQuoted(), true
		}
		return nil, false
	case token.Pipe, token.OrOr:
		return p.rustClosure(), true
	case token.LParen:
		if p.noArrowFn {
			return nil, false
		}
		end := p.matchClose(p.pos)
		if end < 0 || p.tokAt(end+1).Kind != token.FatArrow {
			return nil, false
		}
		return p.arrowFn(end), true
	case token.Ident:
	default:
		return nil, false
	}
	switch {
	case (t.Text == "switch" || t.Text == "when") && sameLine && p.restOfLineHas(token.LBrace) && next.Kind != token.Assign && next.Kind != token.Dot:
		return p.foreignSwitch(), true
	case (t.Text == "try" || t.Text == "do") && next.Kind == token.LBrace:
		return p.tryCatch(), true
	case t.Text == "try" && sameLine && (next.Kind == token.Question || next.Kind == token.Bang) && adjacent(t, next):
		return p.tryOptional(), true
	case t.Text == "try" && sameLine && startsExpr(next.Kind) && next.Kind != token.LBrace:
		p.next()
		x := p.parseUnary()
		d := p.foreign(t.Span, "`try f()` is Swift and Zig syntax; Xo propagates a failure with a postfix `?`: `f()?` (core 2.6)")
		addFix(d, "write `?`", p.deleteToken(t), p.insertAt(x.Span().End, "?"))
		return &ast.TryExpr{Base: ast.Base{Sp: p.spanFrom(t.Span.Start)}, X: x}, true
	case t.Text == "new" && next.Kind == token.Ident && sameLine && isUpper(next.Text):
		d := p.foreign(t.Span, "`new` is TypeScript, Java, and Kotlin syntax; Xo builds a struct with a literal naming every field, `%s{field: value}` (core 2.2)", next.Text)
		// `new P{...}` drops `new`; `new P(...)` needs field names.
		end := p.pos + 2
		for end+1 < len(p.toks) && p.tokAt(end).Kind == token.Dot && p.tokAt(end+1).Kind == token.Ident {
			end += 2
		}
		if p.tokAt(end).Kind == token.LBrace {
			addFix(d, "remove `new`", p.deleteToken(t))
		}
		p.next()
		return p.parseUnary(), true
	case t.Text == "lambda" && (next.Kind == token.Ident || next.Kind == token.Colon):
		return p.pyLambda(), true
	case next.Kind == token.FatArrow && !p.noArrowFn && sameLine:
		return p.arrowFn(-1), true
	case (t.Text == "f" || t.Text == "F") && next.Kind == token.String && adjacent(t, next) && len(next.Text) >= 2 && strings.HasSuffix(next.Text, `"`) && !next.Triple:
		return p.fString(), true
	case next.Kind == token.Bang && adjacent(t, next) && (p.peek(2).Kind == token.LParen || p.peek(2).Kind == token.LBrack) && adjacent(next, p.peek(2)):
		return p.rustMacro(), true
	}
	return nil, false
}

// singleQuoted handles Python and TypeScript `'text'`: the lexer reported
// a rune holding several characters; this names the habit, adds the fix,
// and recovers a string.
func (p *parser) singleQuoted() ast.Expr {
	t := p.tok
	p.next()
	inner := t.Text[1 : len(t.Text)-1]
	var fixed, cooked strings.Builder
	for n := 0; n < len(inner); n++ {
		c := inner[n]
		switch {
		case c == '\\' && n+1 < len(inner):
			e := inner[n+1]
			n++
			switch e {
			case '\'':
				fixed.WriteByte('\'')
				cooked.WriteByte('\'')
				continue
			case 'n':
				cooked.WriteByte('\n')
			case 't':
				cooked.WriteByte('\t')
			default:
				cooked.WriteByte(e)
			}
			fixed.WriteByte('\\')
			fixed.WriteByte(e)
		case c == '"':
			fixed.WriteString(`\"`)
			cooked.WriteByte(c)
		case c == '$' && n+1 < len(inner) && inner[n+1] == '{':
			fixed.WriteString(`\$`)
			cooked.WriteByte(c)
		default:
			fixed.WriteByte(c)
			cooked.WriteByte(c)
		}
	}
	for k := range p.diags.Items {
		d := &p.diags.Items[k]
		if d.Code == string(diag.InvalidRune) && d.Span.Line == t.Span.Start.Line && d.Span.Col == t.Span.Start.Col {
			d.Message = "`'...'` holding text is a Python and TypeScript string; an Xo string uses double quotes, `\"...\"`, and single quotes hold one `Rune` (core 1.6)"
			d.Fixes = append(d.Fixes, diag.Fix{Title: "use double quotes", Edits: []diag.Edit{p.replaceSpan(t.Span, `"`+fixed.String()+`"`)}})
			break
		}
	}
	return strLit(cooked.String(), t.Span)
}

// comprehension is Python's `[elem for pat in iter if cond]`.
type comprehension struct {
	elem ast.Expr
	pat  ast.Pattern
	iter ast.Expr
	cond ast.Expr
	kw   token.Token
}

// parseComprehension parses `for pat in iter [if cond]` after the element.
func (p *parser) parseComprehension(elem ast.Expr) *comprehension {
	c := &comprehension{elem: elem, kw: p.tok}
	p.next() // for
	c.pat = p.parsePattern()
	p.expect(token.In, "`in` in a comprehension")
	c.iter = p.parseBinary(2)
	if p.got(token.If) {
		c.cond = p.parseExpr()
	}
	return c
}

// finishComprehension reports the comprehension and recovers it as
// `iter.filter(fn(x) { cond }).map(fn(x) { elem })`.
func (p *parser) finishComprehension(c *comprehension, sp token.Span) ast.Expr {
	d := p.foreign(c.kw.Span, "`[x for x in xs]` is a Python comprehension; Xo calls the list methods, `xs.filter(fn(x) { cond }).map(fn(x) { f(x) })` (spec/std.md 2)")
	bp, ok := c.pat.(*ast.BindPat)
	if !ok {
		return &ast.ListLit{Base: ast.Base{Sp: sp}, Elems: []ast.Expr{c.elem}}
	}
	name := bp.Name.Name
	param := func() []*ast.Param {
		return []*ast.Param{{Base: ast.Base{Sp: bp.Sp}, Name: ident(name, bp.Sp)}}
	}
	text := p.operandText(c.iter)
	var x ast.Expr = c.iter
	if c.cond != nil {
		text += ".filter(fn(" + name + ") { " + p.spanText(c.cond.Span()) + " })"
		x = methodCall(x, "filter", sp, &ast.FnLit{Base: ast.Base{Sp: c.cond.Span()}, Params: param(), Body: exprBlock(c.cond)})
	}
	if id, ok := c.elem.(*ast.Ident); !ok || id.Name != name {
		text += ".map(fn(" + name + ") { " + p.spanText(c.elem.Span()) + " })"
		x = methodCall(x, "map", sp, &ast.FnLit{Base: ast.Base{Sp: c.elem.Span()}, Params: param(), Body: exprBlock(c.elem)})
	}
	addFix(d, "write `.filter(...).map(...)`", p.replaceSpan(sp, text))
	return x
}

// startsExprOrStmt reports whether k can start the body of a braceless
// `if (c) body`.
func startsExprOrStmt(k token.Kind) bool {
	return startsExpr(k) || k == token.Return || k == token.Break || k == token.Continue
}

// bracelessIf parses Kotlin, Java, and C's `if (c) a else b` (no braces)
// after the parenthesized condition.
func (p *parser) bracelessIf(e *ast.IfExpr, start token.Pos) ast.Expr {
	var edits []diag.Edit
	body := func() *ast.Block {
		s := p.parseSimpleStmt()
		edits = append(edits, p.insertAt(s.Span().Start, "{ "), p.insertAt(s.Span().End, " }"))
		return &ast.Block{Base: ast.Base{Sp: s.Span()}, Stmts: []ast.Stmt{s}}
	}
	thenStart := p.tok
	e.Then = body()
	if p.at(token.Else) {
		p.next()
		switch {
		case p.at(token.If):
			e.Else = p.parseIf()
		case p.at(token.LBrace):
			e.Else = p.parseBlock()
		default:
			e.Else = body()
		}
	}
	e.Sp = p.spanFrom(start)
	d := p.foreign(thenStart.Span, "`if (c) a else b` without braces is Kotlin, Java, and C syntax; every Xo `if` and `else` body is a block: `if c { a } else { b }` (core 3.5)")
	addFix(d, "add braces", edits...)
	return e
}

// rustClosure parses `|a, b| body` and `|| body`.
func (p *parser) rustClosure() ast.Expr {
	open := p.tok
	start := open.Span.Start
	var params []*ast.Param
	var closeTok token.Token
	if p.at(token.OrOr) {
		closeTok = open
		p.next()
	} else {
		p.next()
		for !p.at(token.Pipe) && !p.at(token.EOF) && !p.bad {
			ps := p.tok.Span.Start
			prm := &ast.Param{Name: p.expectIdent("a closure parameter")}
			if p.got(token.Colon) {
				prm.Type = p.parseType()
			}
			prm.Sp = p.spanFrom(ps)
			params = append(params, prm)
			if !p.got(token.Comma) {
				break
			}
		}
		closeTok = p.tok
		p.expect(token.Pipe, "`|` after the closure parameters")
	}
	var names []string
	for _, prm := range params {
		names = append(names, p.spanText(prm.Sp))
	}
	head := "fn(" + strings.Join(names, ", ") + ")"
	f := &ast.FnLit{Params: params}
	d := p.foreign(open.Span.To(closeTok.Span), "`|x| body` is a Rust closure; Xo writes `fn(x) { body }` (core 3.3)")
	if p.at(token.LBrace) {
		f.Body = p.parseBlock()
		addFix(d, "write `fn(...)`", p.replaceSpan(open.Span.To(closeTok.Span), head))
	} else {
		x := p.parseExpr()
		f.Body = exprBlock(x)
		addFix(d, "write `fn(...) { ... }`", p.replaceRange(open.Span.Start, x.Span().Start, head+" { "), p.insertAt(x.Span().End, " }"))
	}
	f.Sp = p.spanFrom(start)
	return f
}

// arrowFn parses `(a, b) => body` (close is the index of `)`) or, with
// close < 0, `x => body`.
func (p *parser) arrowFn(close int) ast.Expr {
	start := p.tok.Span.Start
	var params []*ast.Param
	var headEnd token.Pos
	if close < 0 {
		id := p.expectIdent("a parameter")
		params = []*ast.Param{{Base: ast.Base{Sp: id.Sp}, Name: id}}
		headEnd = id.Sp.End
	} else {
		params = p.parseParams(paramClosure, false)
		headEnd = p.tokAt(p.pos - 1).Span.End
	}
	arrow := p.tok
	p.next() // =>
	var names []string
	for _, prm := range params {
		names = append(names, p.spanText(prm.Sp))
	}
	head := "fn(" + strings.Join(names, ", ") + ")"
	f := &ast.FnLit{Params: params}
	d := p.foreign(arrow.Span, "`(x) => body` is a TypeScript arrow function; Xo writes `fn(x) { body }` (core 3.3)")
	_ = headEnd
	if p.at(token.LBrace) && !p.braceIsMap() {
		f.Body = p.parseBlock()
		addFix(d, "write `fn(...)`", p.replaceRange(start, f.Body.Sp.Start, head+" "))
	} else {
		x := p.parseExpr()
		f.Body = exprBlock(x)
		addFix(d, "write `fn(...) { ... }`", p.replaceRange(start, x.Span().Start, head+" { "), p.insertAt(x.Span().End, " }"))
	}
	f.Sp = p.spanFrom(start)
	return f
}

// pyLambda parses `lambda a, b: body`.
func (p *parser) pyLambda() ast.Expr {
	t := p.tok
	start := t.Span.Start
	p.next()
	var params []*ast.Param
	for p.at(token.Ident) {
		id := p.expectIdent("a parameter")
		params = append(params, &ast.Param{Base: ast.Base{Sp: id.Sp}, Name: id})
		if !p.got(token.Comma) {
			break
		}
	}
	colon := p.tok
	p.expect(token.Colon, "`:` after the lambda parameters")
	x := p.parseExpr()
	var names []string
	for _, prm := range params {
		names = append(names, prm.Name.Name)
	}
	d := p.foreign(t.Span, "`lambda x: body` is Python syntax; Xo writes `fn(x) { body }` (core 3.3)")
	addFix(d, "write `fn(...) { ... }`", p.replaceRange(start, x.Span().Start, "fn("+strings.Join(names, ", ")+") { "), p.insertAt(x.Span().End, " }"))
	_ = colon
	return &ast.FnLit{Base: ast.Base{Sp: p.spanFrom(start)}, Params: params, Body: exprBlock(x)}
}

// fString parses Python's `f"..{x}.."` as an interpolated string.
func (p *parser) fString() ast.Expr {
	ft := p.tok
	p.next()
	st := p.tok
	raw := p.spanText(st.Span)
	d := p.foreign(ft.Span.To(st.Span), "`f\"...{x}...\"` is a Python f-string; every Xo string interpolates with `${x}`: `\"...${x}...\"` (core 1.6)")
	// Rewrite the literal: `{expr}` becomes `${expr}`, `{{` and `}}` become
	// literal braces. Format specs (`{x:.2f}`, `{x!r}`) have no Xo form.
	var b strings.Builder
	var parts []ast.StringPart
	var lit strings.Builder
	ok := !strings.HasPrefix(raw, `"""`)
	contentStart := st.Span.Start.Offset + 1
	inner := raw[1 : len(raw)-1]
	for i := 0; ok && i < len(inner); i++ {
		c := inner[i]
		switch {
		case c == '{' && i+1 < len(inner) && inner[i+1] == '{':
			b.WriteByte('{')
			lit.WriteByte('{')
			i++
		case c == '}' && i+1 < len(inner) && inner[i+1] == '}':
			b.WriteByte('}')
			lit.WriteByte('}')
			i++
		case c == '{':
			j := strings.IndexByte(inner[i:], '}')
			if j < 0 {
				ok = false
				break
			}
			expr := inner[i+1 : i+j]
			if strings.ContainsAny(expr, ":!") && !strings.Contains(expr, "::") {
				ok = false
				break
			}
			b.WriteString("${" + expr + "}")
			if lit.Len() > 0 {
				parts = append(parts, ast.StringPart{Text: lit.String()})
				lit.Reset()
			}
			off := contentStart + i + 1
			pos := token.Pos{Line: st.Span.Start.Line, Col: st.Span.Start.Col + 1 + i + 1, Offset: off}
			end := token.Pos{Line: pos.Line, Col: pos.Col + len(expr), Offset: off + len(expr)}
			e := p.parseInterp(token.StrPart{IsExpr: true, Text: expr, Span: token.Span{File: p.file, Start: pos, End: end}})
			parts = append(parts, ast.StringPart{Expr: e})
			i += j
		case c == '$':
			b.WriteString(`\$`)
			lit.WriteByte('$')
		case c == '\\' && i+1 < len(inner):
			b.WriteByte(c)
			b.WriteByte(inner[i+1])
			lit.WriteByte(inner[i+1])
			i++
		default:
			b.WriteByte(c)
			lit.WriteByte(c)
		}
	}
	p.next()
	if ok {
		addFix(d, "write `${...}`", p.replaceSpan(ft.Span.To(st.Span), `"`+b.String()+`"`))
	} else {
		addNote(d, "format specs such as `{x:.2f}` have no Xo form; format the value first")
	}
	if lit.Len() > 0 {
		parts = append(parts, ast.StringPart{Text: lit.String()})
	}
	return &ast.StringLit{Base: ast.Base{Sp: ft.Span.To(st.Span)}, Parts: parts}
}

// rustMacro handles `vec![...]`, `format!(...)`, `panic!(...)`,
// `println!(...)` and other Rust macro calls.
func (p *parser) rustMacro() ast.Expr {
	name := p.tok
	bang := p.peek(1)
	start := name.Span.Start
	p.next()
	p.next()
	switch name.Text {
	case "vec":
		x := p.parsePrimary()
		d := p.foreign(name.Span.To(bang.Span), "`vec![...]` is a Rust macro; an Xo list literal is `[...]` (core 1.6)")
		if _, ok := x.(*ast.ListLit); ok {
			addFix(d, "remove `vec!`", p.replaceSpan(name.Span.To(bang.Span), ""))
		}
		return x
	case "panic", "unreachable", "todo", "unimplemented":
		args := p.macroArgs()
		d := p.foreign(name.Span.To(bang.Span), "`%s!` is a Rust macro; Xo stops the program on a broken invariant with `fault(\"why\")` (core 8)", name.Text)
		if len(args) == 1 {
			addFix(d, "write `fault(...)`", p.replaceSpan(name.Span.To(bang.Span), "fault"))
		}
		call := &ast.CallExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Fun: ident("fault", name.Span)}
		for _, a := range args {
			call.Args = append(call.Args, &ast.Arg{Base: ast.Base{Sp: a.Span()}, Value: a})
		}
		if len(args) == 0 {
			call.Args = []*ast.Arg{{Base: ast.Base{Sp: name.Span}, Value: strLit(name.Text, name.Span)}}
		}
		return call
	case "format":
		args := p.macroArgs()
		d := p.foreign(name.Span.To(bang.Span), "`format!` is a Rust macro; an Xo string interpolates directly: `\"${a} and ${b}\"` (core 1.6)")
		return p.formatString(d, name, args)
	case "assert", "assert_eq", "assert_ne":
		args := p.macroArgs()
		d := p.foreign(name.Span.To(bang.Span), "`%s!` is a Rust macro; an Xo test checks with `expect(x) == y` or `expect(cond)`, and a function states conditions with `requires` and `ensures` (core 9.1, 9.2)", name.Text)
		if strings.HasSuffix(p.file, "_test.xo") {
			sp := p.spanFrom(start)
			switch {
			case name.Text == "assert" && len(args) == 1:
				addFix(d, "write `expect(...)`", p.replaceSpan(name.Span.To(bang.Span), "expect"))
			case name.Text == "assert_eq" && len(args) == 2:
				addFix(d, "write `expect(a) == b`", p.replaceSpan(sp, "expect("+p.spanText(args[0].Span())+") == "+p.spanText(args[1].Span())))
			case name.Text == "assert_ne" && len(args) == 2:
				addFix(d, "write `expect(a) != b`", p.replaceSpan(sp, "expect("+p.spanText(args[0].Span())+") != "+p.spanText(args[1].Span())))
			}
		}
		return &ast.TupleExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Elems: args}
	}
	args := p.macroArgs()
	if name.Text == "println" || name.Text == "print" || name.Text == "eprintln" || name.Text == "eprint" {
		p.foreign(name.Span.To(bang.Span), "`%s!` is a Rust macro; Xo prints through the `Stdio` capability, `os.stdio.println(\"${x}\")` in `main`, or a `stdio: Stdio` parameter elsewhere; `dbg(x)` prints a value while debugging (spec/std.md 3, core 11.4)", name.Text)
	} else {
		p.foreign(name.Span.To(bang.Span), "`%s!` is a Rust macro; Xo has no macros (core 0)", name.Text)
	}
	return &ast.TupleExpr{Base: ast.Base{Sp: p.spanFrom(start)}, Elems: args}
}

// macroArgs parses the parenthesized arguments of a macro call.
func (p *parser) macroArgs() []ast.Expr {
	var args []ast.Expr
	if p.at(token.LParen) || p.at(token.LBrack) {
		closeK := token.RParen
		if p.at(token.LBrack) {
			closeK = token.RBrack
		}
		p.parseList(p.tok.Kind, closeK, "argument", func() {
			args = append(args, p.parseExpr())
		})
	}
	return args
}

// formatString turns Rust's `format!("{} {}", a, b)` into an interpolated
// string, with a fix when every placeholder is a plain `{}` or `{name}`.
func (p *parser) formatString(d *diag.Diagnostic, name token.Token, args []ast.Expr) ast.Expr {
	sp := p.spanFrom(name.Span.Start)
	if len(args) == 0 {
		return strLit("", sp)
	}
	lit, ok := args[0].(*ast.StringLit)
	if !ok || lit.Kind != ast.PlainString || len(lit.Parts) > 1 || (len(lit.Parts) == 1 && lit.Parts[0].Expr != nil) {
		return &ast.TupleExpr{Base: ast.Base{Sp: sp}, Elems: args}
	}
	text := ""
	if len(lit.Parts) == 1 {
		text = lit.Parts[0].Text
	}
	rest := args[1:]
	var out strings.Builder
	var parts []ast.StringPart
	var cur strings.Builder
	fixable := true
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '{' && i+1 < len(text) && text[i+1] == '{':
			out.WriteByte('{')
			cur.WriteByte('{')
			i++
		case c == '}' && i+1 < len(text) && text[i+1] == '}':
			out.WriteByte('}')
			cur.WriteByte('}')
			i++
		case c == '{':
			j := strings.IndexByte(text[i:], '}')
			if j < 0 {
				fixable = false
				break
			}
			inner := text[i+1 : i+j]
			if cur.Len() > 0 {
				parts = append(parts, ast.StringPart{Text: cur.String()})
				cur.Reset()
			}
			switch {
			case inner == "" && len(rest) > 0:
				out.WriteString("${" + p.spanText(rest[0].Span()) + "}")
				parts = append(parts, ast.StringPart{Expr: rest[0]})
				rest = rest[1:]
			case inner != "" && !strings.ContainsAny(inner, ":?"):
				out.WriteString("${" + inner + "}")
				parts = append(parts, ast.StringPart{Expr: ident(inner, lit.Sp)})
			default:
				fixable = false
			}
			i += j
		case c == '"' || c == '\\' || c == '$':
			out.WriteByte('\\')
			out.WriteByte(c)
			cur.WriteByte(c)
		case c == '\n':
			out.WriteString(`\n`)
			cur.WriteByte(c)
		case c == '\t':
			out.WriteString(`\t`)
			cur.WriteByte(c)
		default:
			out.WriteByte(c)
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, ast.StringPart{Text: cur.String()})
	}
	if fixable && len(rest) == 0 {
		addFix(d, "write an interpolated string", p.replaceSpan(sp, `"`+out.String()+`"`))
		return &ast.StringLit{Base: ast.Base{Sp: sp}, Parts: parts}
	}
	return &ast.TupleExpr{Base: ast.Base{Sp: sp}, Elems: args}
}

// tryCatch recovers `try { ... } catch (e) { ... }` (and Swift's `do`)
// by skipping it: Xo has no exceptions, so there is no faithful rewrite.
func (p *parser) tryCatch() ast.Expr {
	t := p.tok
	start := t.Span.Start
	end := p.matchClose(p.pos + 1)
	if end < 0 {
		p.errorf(diag.UnclosedDelimiter, p.peek(1).Span, "`{` is never closed")
		return &ast.BadExpr{Base: ast.Base{Sp: t.Span}}
	}
	// catch clauses and finally.
	for {
		k := p.tokAt(end + 1)
		if k.Kind != token.Ident || (k.Text != "catch" && k.Text != "except" && k.Text != "finally") {
			break
		}
		j := end + 2
		for j < len(p.toks) && p.tokAt(j).Kind != token.LBrace && p.tokAt(j).Kind != token.Newline && p.tokAt(j).Kind != token.EOF {
			j++
		}
		if p.tokAt(j).Kind != token.LBrace {
			break
		}
		e := p.matchClose(j)
		if e < 0 {
			break
		}
		end = e
	}
	p.foreign(t.Span, "`%s { ... } catch` is TypeScript, Kotlin, Python, and Swift syntax; Xo has no exceptions: a call that can fail returns a Result, so propagate it with `f()?` or handle it with `match f() { Ok(v) => ...  Err(e) => ... }` (core 2.6)", t.Text)
	p.skipTo(end + 1)
	return &ast.BadExpr{Base: ast.Base{Sp: p.spanFrom(start)}}
}

// tryOptional handles Swift's `try?` and `try!`.
func (p *parser) tryOptional() ast.Expr {
	t := p.tok
	mark := p.peek(1)
	start := t.Span.Start
	p.next()
	p.next()
	x := p.parseUnary()
	if mark.Kind == token.Question {
		d := p.foreign(t.Span.To(mark.Span), "`try? f()` is Swift syntax; Xo turns a failure into `None` with `f().ok()` (spec/std.md 2)")
		addFix(d, "write `.ok()`", p.replaceRange(t.Span.Start, p.afterBlanks(mark.Span.End), ""), p.insertAt(x.Span().End, ".ok()"))
		return methodCall(x, "ok", p.spanFrom(start))
	}
	p.foreign(t.Span.To(mark.Span), "`try! f()` is Swift syntax; Xo stops on an impossible failure with `f().or_fault(\"why it cannot fail\")` (spec/std.md 2)")
	return methodCall(x, "or_fault", p.spanFrom(start), strLit(orFaultReason, t.Span))
}

// foreignPostfix handles postfix forms from other languages after x:
// `x as T`, `x!!`, `x!`, `A::B`. ok is false when none applies; stop is
// set when the postfix loop must end (a ternary or elvis follows).
func (p *parser) foreignPostfix(x ast.Expr) (y ast.Expr, ok, stop bool) {
	t := p.tok
	next := p.peek(1)
	switch t.Kind {
	case token.Question:
		if next.Kind == token.Colon && adjacent(t, next) {
			return x, false, true
		}
		if p.ternaryAhead() {
			return x, false, true
		}
	case token.As:
		p.next()
		typ := p.parseType()
		d := p.foreign(t.Span, "`x as T` is Rust, TypeScript, and Kotlin syntax; Xo converts with a call on the target type, `T(x)`, and has no unchecked casts (core 2.1)")
		nt, isNamed := typ.(*ast.NamedType)
		if !isNamed || len(nt.Args) > 0 {
			return x, true, false
		}
		addFix(d, "write `"+p.spanText(typ.Span())+"(...)`", p.insertAt(x.Span().Start, p.spanText(typ.Span())+"("), p.replaceRange(x.Span().End, typ.Span().End, ")"))
		var fun ast.Expr = nt.Path[0]
		for _, seg := range nt.Path[1:] {
			fun = &ast.SelectorExpr{Base: ast.Base{Sp: fun.Span().To(seg.Sp)}, X: fun, Sel: seg}
		}
		sp := x.Span().To(typ.Span())
		return &ast.CallExpr{Base: ast.Base{Sp: sp}, Fun: fun, Args: []*ast.Arg{{Base: ast.Base{Sp: x.Span()}, Value: x}}}, true, false
	case token.Bang:
		if !adjacent(p.tokAt(p.pos-1), t) {
			return x, false, false
		}
		end := t
		double := next.Kind == token.Bang && adjacent(t, next)
		if double {
			end = next
		} else if startsExpr(next.Kind) && next.Kind != token.LParen && next.Kind != token.LBrack {
			return x, false, false
		}
		lang := "Swift and TypeScript"
		if double {
			lang = "Kotlin"
		}
		p.foreign(t.Span.To(end.Span), "`%s` force unwraps in %s; Xo says why the value cannot be missing, `x.or_fault(\"why\")`, or handles the missing case with `?`, `??`, `if let`, or `match` (core 2.5)", p.spanText(t.Span.To(end.Span)), lang)
		p.next()
		if double {
			p.next()
		}
		return methodCall(x, "or_fault", x.Span().To(end.Span), strLit(orFaultReason, t.Span)), true, false
	case token.Colon:
		if next.Kind != token.Colon || !adjacent(t, next) || p.peek(2).Kind != token.Ident || !adjacent(next, p.peek(2)) || !isPath(x) {
			return x, false, false
		}
		if id, ok := x.(*ast.Ident); !ok || id.Name != "String" {
			// `String::from(s)` is reported whole by the checker.
			d := p.foreign(t.Span.To(next.Span), "`::` is Rust and C++ path syntax; Xo writes `.`: `%s.%s` (core 2.3, 4.1)", p.spanText(x.Span()), p.peek(2).Text)
			addFix(d, "write `.`", p.replaceSpan(t.Span.To(next.Span), "."))
		}
		p.next()
		p.next()
		sel := p.expectIdent("a name after `::`")
		return &ast.SelectorExpr{Base: ast.Base{Sp: x.Span().To(sel.Sp)}, X: x, Sel: sel}, true, false
	}
	return x, false, false
}

// foreignSwitch parses `switch x { case 1: ... default: ... }` (Go,
// Swift, TypeScript) and Kotlin's `when (x) { 1 -> ...  else -> ... }` as a
// match.
func (p *parser) foreignSwitch() ast.Expr {
	kw := p.tok
	start := kw.Span.Start
	p.next()
	kotlin := kw.Text == "when"
	var subject ast.Expr
	if !p.at(token.LBrace) {
		subject = p.parseExprNoLit()
		if pe, ok := subject.(*ast.ParenExpr); ok {
			subject = pe.X
		}
	}
	if !p.at(token.LBrace) {
		p.errorExpected("`{`")
		return &ast.BadExpr{Base: ast.Base{Sp: kw.Span}}
	}
	open := p.tok
	m := &ast.MatchExpr{Subject: subject}
	var edits []diag.Edit
	fixable := subject != nil
	if subject != nil {
		edits = append(edits, p.replaceRange(kw.Span.Start, open.Span.Start, "match "+p.spanText(subject.Span())+" "))
	}
	p.next() // {
	for {
		p.skipNewlines()
		if p.at(token.RBrace) || p.at(token.EOF) || p.bad {
			break
		}
		armStart := p.tok
		arm := &ast.MatchArm{}
		var pats []ast.Pattern
		isDefault := false
		switch {
		case !kotlin && p.atIdent("case"):
			p.next()
			for {
				pats = append(pats, p.parsePattern())
				if !p.got(token.Comma) {
					break
				}
			}
		case !kotlin && p.atIdent("default"):
			isDefault = true
			p.next()
		case kotlin && p.at(token.Else):
			isDefault = true
			p.next()
		case kotlin:
			for {
				pats = append(pats, p.parsePattern())
				if !p.got(token.Comma) {
					break
				}
			}
		default:
			p.errorExpected("`case` or `default`")
		}
		if p.bad {
			break
		}
		sep := p.tok
		if kotlin {
			p.expect(token.Arrow, "`->` after the `when` branch condition")
		} else {
			p.expect(token.Colon, "`:` after the case")
		}
		if p.bad {
			break
		}
		if isDefault {
			arm.Pattern = &ast.WildcardPat{Base: ast.Base{Sp: armStart.Span}}
		} else if len(pats) == 1 {
			arm.Pattern = pats[0]
		} else {
			arm.Pattern = &ast.OrPat{Base: ast.Base{Sp: pats[0].Span().To(pats[len(pats)-1].Span())}, Alts: pats}
		}
		// Arm head edit.
		head := "_ =>"
		if !isDefault {
			var texts []string
			for _, pt := range pats {
				texts = append(texts, p.spanText(pt.Span()))
			}
			head = strings.Join(texts, " | ") + " =>"
		}
		// Body: Kotlin takes one expression or block; switch takes the
		// statements up to the next case.
		var stmts []ast.Stmt
		if !kotlin {
			p.skipNewlines()
		}
		bodyStart := p.tok.Span.Start
		if kotlin {
			if p.at(token.LBrace) {
				b := p.parseBlock()
				stmts = []ast.Stmt{&ast.ExprStmt{Base: b.Base, X: b}}
			} else {
				stmts = []ast.Stmt{p.parseSimpleStmt()}
			}
		} else {
			for {
				p.skipNewlines()
				if p.at(token.RBrace) || p.at(token.EOF) || p.atIdent("case") || p.atIdent("default") || p.bad {
					break
				}
				if p.atIdent("fallthrough") || p.at(token.Break) {
					fixable = false
				}
				stmts = append(stmts, p.parseStmt())
				if !p.got(token.Newline) && !p.at(token.RBrace) {
					break
				}
			}
		}
		switch len(stmts) {
		case 0:
			fixable = false
			arm.Body = &ast.ExprStmt{Base: ast.Base{Sp: sep.Span}, X: &ast.UnitExpr{Base: ast.Base{Sp: sep.Span}}}
		case 1:
			arm.Body = stmts[0]
			edits = append(edits, p.replaceRange(armStart.Span.Start, bodyStart, head+" "))
		default:
			sp := stmts[0].Span().To(stmts[len(stmts)-1].Span())
			b := &ast.Block{Base: ast.Base{Sp: sp}, Stmts: stmts}
			arm.Body = &ast.ExprStmt{Base: b.Base, X: b}
			edits = append(edits, p.replaceRange(armStart.Span.Start, bodyStart, head+" {\n"), p.insertAt(sp.End, "\n}"))
		}
		arm.Sp = p.spanFrom(armStart.Span.Start)
		m.Arms = append(m.Arms, arm)
	}
	closeTok := p.tok
	p.closeBrace(open)
	m.Sp = p.spanFrom(start)
	lang := "Go, Swift, and TypeScript"
	form := "`switch x { case 1: ... default: ... }`"
	if kotlin {
		lang, form = "Kotlin", "`when (x) { 1 -> ...  else -> ... }`"
	}
	d := p.foreign(kw.Span, "%s is %s syntax; Xo writes `match x { 1 => ...  _ => ... }`: one `pattern => body` arm per line, `_` for the default, `1 | 2` for several values, no fallthrough, and every case covered (core 3.5)", form, lang)
	_ = closeTok
	if fixable {
		addFix(d, "write `match`", edits...)
	} else if subject == nil {
		addNote(d, "a `switch` with no subject is an `if ... else if ...` chain in Xo")
	}
	if subject == nil {
		return &ast.BadExpr{Base: ast.Base{Sp: m.Sp}}
	}
	return m
}

// foreignSlice handles `xs[a:b]` (Go, Python) inside an index, with the
// low bound (nil for `xs[:b]`) already parsed.
func (p *parser) foreignSlice(lo ast.Expr) ast.Expr {
	colon := p.tok
	start := colon.Span.Start
	if lo != nil {
		start = lo.Span().Start
	}
	p.next()
	r := &ast.RangeExpr{Lo: lo}
	if !p.at(token.RBrack) {
		r.Hi = p.parseBinary(rangePrec + 1)
	}
	r.Sp = p.spanFrom(start)
	d := p.foreign(colon.Span, "`xs[a:b]` is Go and Python slice syntax; Xo slices with a range, `xs[a..b]` (core 3.6)")
	addFix(d, "write `..`", p.replaceSpan(colon.Span, ".."))
	return r
}

// foreignElif handles `elif`, `elsif`, and `elseif` after an `if` block.
func (p *parser) foreignElif() bool {
	t := p.tok
	if t.Kind != token.Ident || (t.Text != "elif" && t.Text != "elsif" && t.Text != "elseif") {
		return false
	}
	lang := map[string]string{"elif": "Python", "elsif": "Ruby and Perl", "elseif": "PHP and Lua"}[t.Text]
	d := p.foreign(t.Span, "`%s` is %s syntax; Xo writes `else if` (core 3.5)", t.Text, lang)
	addFix(d, "write `else if`", p.replaceSpan(t.Span, "else if"))
	return true
}

// armComma reports XO0169 for a `,` after a match or select arm (Rust,
// Kotlin) with a fix, and consumes it.
func (p *parser) armComma(what string) {
	c := p.tok
	d := p.softErrorf(diag.MatchArmSeparator, c.Span, "%ss are separated by newlines, not `,`", what)
	addNote(d, "Rust and Kotlin separate arms with `,`; an Xo arm ends at its newline (core 3.5)")
	next := p.peek(1)
	if next.Kind != token.Newline && next.Kind != token.RBrace && next.Kind != token.EOF && next.Span.Start.Line == c.Span.Start.Line {
		addFix(d, "put the next arm on its own line", p.replaceRange(c.Span.Start, next.Span.Start, "\n"))
	} else {
		addFix(d, "remove `,`", p.replaceSpan(c.Span, ""))
	}
	p.next()
}
