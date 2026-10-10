package parser

import (
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// A line that starts with a binary operator (core 1.3, XO0178). The lexer
// ends a statement at a newline after an identifier, literal, `)`, `]`,
// `}`, or `?`, so a condition or sum continued on the next line with a
// leading `||`, `&&`, or `+` (the style of Rust, Swift, Kotlin, and some
// Go code) leaves an operator at the start of a statement. Before this
// check the parser saw `|| ...` there as a Rust closure (XO0177) and
// cascaded (XO0160, XO0175). Like the other token habits it runs before
// parsing: it reports the line once, with a fix that moves the operator
// to the end of the previous line, and drops the newline so the parser
// reads the expression as continued.

// leadingOperator reports whether the newline token at i is followed by
// a binary operator that starts the next line, and if so reports XO0178.
func (p *parser) leadingOperator(i int) bool {
	if i == 0 || i+1 >= len(p.toks) {
		return false
	}
	prev, op, next := p.tokAt(i-1), p.tokAt(i+1), p.tokAt(i+2)
	if !binaryOnly(op, next) || op.Span.Start.Line <= prev.Span.End.Line {
		return false
	}
	switch prev.Kind {
	case token.Return, token.Break, token.Continue:
		// `return` ends its statement; the operator does not continue it
		return false
	}
	if p.lineHasArrow(i + 2) {
		// `* => 3`: a match arm with a bad pattern, not a continuation
		return false
	}
	text := op.Kind.String()
	d := p.diags.Errorf(diag.LeadingOperator, op.Span, "a line cannot start with the binary operator `%s`; end the previous line with it (core 1.3)", text)
	addNote(d, "a newline after an identifier, literal, `)`, `]`, `}`, or `?` ends the statement, so `%s` here would begin a new one", text)
	del := p.replaceSpan(op.Span, "")
	if next.Kind != token.Newline && next.Kind != token.EOF && next.Span.Start.Line == op.Span.End.Line {
		del = p.replaceRange(op.Span.Start, next.Span.Start, "")
	}
	addFix(d, "move `"+text+"` to the end of the previous line", p.insertAt(prev.Span.End, " "+text), del)
	return true
}

// binaryOnly reports whether op, followed by next, can only be a binary
// operator: not `-`, `!`, `~`, or `..` (which start expressions), and
// not `*` or `&` written against their operand (Go's `*p` and `&x`,
// reported by their own habits), nor `++`.
func binaryOnly(op, next token.Token) bool {
	switch op.Kind {
	case token.OrOr, token.AndAnd, token.Coalesce, token.Pipe, token.Caret, token.Slash, token.Percent,
		token.Shl, token.Shr, token.Eq, token.NotEq, token.Lt, token.LtEq, token.Gt, token.GtEq:
		return true
	case token.Plus:
		return !(next.Kind == token.Plus && adjacent(op, next))
	case token.Star, token.Amp:
		return !adjacent(op, next)
	}
	return false
}

// lineHasArrow reports whether a `=>` outside brackets follows token j on
// its line.
func (p *parser) lineHasArrow(j int) bool {
	depth := 0
	for ; j < len(p.toks); j++ {
		switch p.tokAt(j).Kind {
		case token.Newline, token.EOF:
			if depth == 0 {
				return false
			}
		case token.LParen, token.LBrack, token.LBrace, token.HashLBrace:
			depth++
		case token.RParen, token.RBrack, token.RBrace:
			if depth == 0 {
				return false
			}
			depth--
		case token.FatArrow:
			if depth == 0 {
				return true
			}
		}
		if p.tokAt(j).Kind == token.EOF {
			return false
		}
	}
	return false
}
