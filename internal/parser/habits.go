package parser

// Habit diagnostics (decision 0087): syntax that is correct in Go, Rust,
// TypeScript, Python, Swift, or Kotlin but not in Xo is reported once, as
// XO0177, naming the habit and the Xo form, with a machine fix when the
// rewrite is unambiguous. The parser then recovers as if the Xo form had
// been written, so the checker sees the intended program and one habit
// produces one diagnostic, never a cascade.

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// foreign reports XO0177 at sp. It is a soft error: the caller recovers
// locally. It returns nil when an error is already pending.
func (p *parser) foreign(sp token.Span, format string, args ...any) *diag.Diagnostic {
	return p.softErrorf(diag.ForeignSyntax, sp, format, args...)
}

// addFix attaches a machine fix to d (nil safe).
func addFix(d *diag.Diagnostic, title string, edits ...diag.Edit) {
	if d == nil || len(edits) == 0 {
		return
	}
	d.Fixes = append(d.Fixes, diag.Fix{Title: title, Edits: edits})
}

// addNote attaches a note to d (nil safe).
func addNote(d *diag.Diagnostic, format string, args ...any) {
	if d == nil {
		return
	}
	d.Notes = append(d.Notes, diag.NoteItem{Message: fmt.Sprintf(format, args...)})
}

// replaceRange replaces the source between a and b.
func (p *parser) replaceRange(a, b token.Pos, text string) diag.Edit {
	return diag.Edit{Span: diag.FromSpan(token.Span{File: p.file, Start: a, End: b}), Replace: text}
}

// replaceSpan replaces the source of sp.
func (p *parser) replaceSpan(sp token.Span, text string) diag.Edit {
	return p.replaceRange(sp.Start, sp.End, text)
}

// insertAt inserts text at pos.
func (p *parser) insertAt(pos token.Pos, text string) diag.Edit {
	return diag.Edit{Span: diag.PointSpan(p.file, pos), Replace: text}
}

// srcText returns the source between a and b.
func (p *parser) srcText(a, b token.Pos) string {
	if a.Offset < 0 || b.Offset > len(p.src) || a.Offset > b.Offset {
		return ""
	}
	return string(p.src[a.Offset:b.Offset])
}

// spanText returns the source of sp.
func (p *parser) spanText(sp token.Span) string { return p.srcText(sp.Start, sp.End) }

// adjacent reports whether b starts exactly where a ends (no blank between).
func adjacent(a, b token.Token) bool { return a.Span.End.Offset == b.Span.Start.Offset }

// lineStart is the position of the first byte of pos's line.
func (p *parser) lineStart(pos token.Pos) token.Pos {
	return token.Pos{Line: pos.Line, Col: 1, Offset: pos.Offset - (pos.Col - 1)}
}

// nextLineStart is the position just after the newline that ends pos's
// line (or the end of the file).
func (p *parser) nextLineStart(pos token.Pos) token.Pos {
	i := strings.IndexByte(string(p.src[pos.Offset:]), '\n')
	if i < 0 {
		end := len(p.src)
		return token.Pos{Line: pos.Line, Col: pos.Col + (end - pos.Offset), Offset: end}
	}
	return token.Pos{Line: pos.Line + 1, Col: 1, Offset: pos.Offset + i + 1}
}

// withBlank extends a deletion that starts at t to also remove the blanks
// that follow t, so `var x` becomes `x` rather than ` x`.
func (p *parser) afterBlanks(pos token.Pos) token.Pos {
	off := pos.Offset
	col := pos.Col
	for off < len(p.src) && (p.src[off] == ' ' || p.src[off] == '\t') {
		off++
		col++
	}
	return token.Pos{Line: pos.Line, Col: col, Offset: off}
}

// beforeBlanks moves pos back over blanks on its line.
func (p *parser) beforeBlanks(pos token.Pos) token.Pos {
	off := pos.Offset
	col := pos.Col
	for off > 0 && (p.src[off-1] == ' ' || p.src[off-1] == '\t') {
		off--
		col--
	}
	return token.Pos{Line: pos.Line, Col: col, Offset: off}
}

// deleteToken returns an edit removing t and the blanks after it.
func (p *parser) deleteToken(t token.Token) diag.Edit {
	return p.replaceRange(t.Span.Start, p.afterBlanks(t.Span.End), "")
}

// startsExpr reports whether k can start an expression operand.
func startsExpr(k token.Kind) bool {
	switch k {
	case token.Ident, token.Int, token.Float, token.Duration, token.Rune, token.String,
		token.RawString, token.Bytes, token.Hole, token.LParen, token.LBrack, token.LBrace,
		token.HashLBrace, token.Minus, token.Bang, token.Tilde, token.Fn, token.If, token.Match:
		return true
	}
	return false
}

// simpleOperand reports whether e can take a method call without
// parentheses: `xs.len()`.
func simpleOperand(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.CallExpr, *ast.IndexExpr, *ast.ParenExpr,
		*ast.StringLit, *ast.ListLit, *ast.MapLit, *ast.SetLit, *ast.TupleExpr, *ast.StructLit:
		return true
	}
	return false
}

// operandText is the source of e, parenthesized when a method call or a
// postfix would otherwise bind to part of it.
func (p *parser) operandText(e ast.Expr) string {
	t := p.spanText(e.Span())
	if simpleOperand(e) {
		return t
	}
	return "(" + t + ")"
}

// sameLine reports whether the token at index i is on the line of tok and
// no newline token separates them.
func (p *parser) restOfLineHas(kind token.Kind) bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.Newline, token.EOF:
			return false
		case token.LParen, token.LBrack, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack:
			depth--
			if depth < 0 {
				return false
			}
		case token.LBrace:
			if depth == 0 && kind == token.LBrace {
				return true
			}
			depth++
		case token.RBrace:
			depth--
			if depth < 0 {
				return false
			}
		}
		if depth == 0 && p.toks[i].Kind == kind && i > p.pos {
			return true
		}
	}
	return false
}

// matchClose returns the index of the token that closes the bracket at
// index i, or -1.
func (p *parser) matchClose(i int) int {
	open := p.toks[i].Kind
	var close token.Kind
	switch open {
	case token.LParen:
		close = token.RParen
	case token.LBrack:
		close = token.RBrack
	case token.LBrace, token.HashLBrace:
		close = token.RBrace
	default:
		return -1
	}
	depth := 0
	for j := i; j < len(p.toks); j++ {
		k := p.toks[j].Kind
		switch {
		case k == open || (close == token.RBrace && (k == token.LBrace || k == token.HashLBrace)):
			depth++
		case k == close:
			depth--
			if depth == 0 {
				return j
			}
		case k == token.EOF:
			return -1
		}
	}
	return -1
}

// skipTo advances until the token index is i (exclusive of nothing: the
// current token becomes toks[i]).
func (p *parser) skipTo(i int) {
	for p.pos < i && !p.at(token.EOF) {
		p.next()
	}
}

// skipLine consumes tokens up to and including the next newline at bracket
// depth zero.
func (p *parser) skipLine() {
	depth := 0
	for !p.at(token.EOF) {
		switch p.tok.Kind {
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			depth--
		case token.Newline:
			if depth <= 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

// ident builds an identifier node.
func ident(name string, sp token.Span) *ast.Ident {
	return &ast.Ident{Base: ast.Base{Sp: sp}, Name: name}
}

// exprBlock wraps an expression in a block, for recovered `if` arms.
func exprBlock(e ast.Expr) *ast.Block {
	return &ast.Block{Base: ast.Base{Sp: e.Span()}, Stmts: []ast.Stmt{&ast.ExprStmt{Base: ast.Base{Sp: e.Span()}, X: e}}}
}

// methodCall builds `x.name(args)` for recovery.
func methodCall(x ast.Expr, name string, sp token.Span, args ...ast.Expr) *ast.CallExpr {
	sel := &ast.SelectorExpr{Base: ast.Base{Sp: sp}, X: x, Sel: ident(name, sp)}
	call := &ast.CallExpr{Base: ast.Base{Sp: sp}, Fun: sel}
	for _, a := range args {
		call.Args = append(call.Args, &ast.Arg{Base: ast.Base{Sp: a.Span()}, Value: a})
	}
	return call
}

// strLit builds a plain string literal for recovery.
func strLit(s string, sp token.Span) *ast.StringLit {
	return &ast.StringLit{Base: ast.Base{Sp: sp}, Parts: []ast.StringPart{{Text: s}}}
}

// orFaultReason is the reason text recovery uses for a force unwrap. It is
// never written to source: force unwraps get no machine fix.
const orFaultReason = "forced unwrap"
