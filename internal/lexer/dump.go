package lexer

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Dump lexes src and prints every token, comment, and diagnostic in the
// canonical text format shared with the self hosted lexer in
// selfhost/lexer (its dump.xo). The two outputs are compared byte for byte
// by TestSelfhostDifferential.
//
// Format, one item per line:
//
//	== <file>
//	<Kind> <span> "<text>" "<value>"[ triple]
//	  lit <span> "<text>"              a literal part of a String token
//	  expr <span> "<text>"             an interpolated part, followed by its
//	    <Kind> ...                     tokens from LexRange, indented deeper
//	comment <span> doc|line "<text>"
//	diag <code> <line>:<col>-<line>:<col> "<message>"
//
// A span is line:col:offset-line:col:offset. Quoted text escapes `"`, `\`,
// and every byte outside printable ASCII as \xHH, so invalid UTF-8 prints
// unambiguously. Diagnostics come last, in the order they were reported:
// first those of Lex, then those of each LexRange in token order.
func Dump(file string, src []byte) string {
	var ds diag.List
	r := Lex(file, src, &ds)
	var b strings.Builder
	b.WriteString("== " + file + "\n")
	dumpTokens(&b, file, src, r.Tokens, "", &ds)
	for _, c := range r.Comments {
		kind := "line"
		if c.Doc {
			kind = "doc"
		}
		fmt.Fprintf(&b, "comment %s %s %s\n", dumpSpan(c.Span), kind, dumpQuote(c.Text))
	}
	for _, d := range ds.Items {
		fmt.Fprintf(&b, "diag %s %d:%d-%d:%d %s\n", d.Code, d.Span.Line, d.Span.Col, d.Span.EndLine, d.Span.EndCol, dumpQuote(d.Message))
	}
	return b.String()
}

func dumpTokens(b *strings.Builder, file string, src []byte, toks []token.Token, indent string, ds *diag.List) {
	for _, t := range toks {
		fmt.Fprintf(b, "%s%s %s %s %s", indent, DumpKindName(t.Kind), dumpSpan(t.Span), dumpQuote(t.Text), dumpQuote(t.Value))
		if t.Triple {
			b.WriteString(" triple")
		}
		b.WriteByte('\n')
		for _, p := range t.Parts {
			if !p.IsExpr {
				fmt.Fprintf(b, "%s  lit %s %s\n", indent, dumpSpan(p.Span), dumpQuote(p.Text))
				continue
			}
			fmt.Fprintf(b, "%s  expr %s %s\n", indent, dumpSpan(p.Span), dumpQuote(p.Text))
			sub := LexRange(file, src, p.Span.Start, p.Span.End.Offset, ds)
			dumpTokens(b, file, src, sub, indent+"    ", ds)
		}
	}
}

func dumpSpan(s token.Span) string {
	return fmt.Sprintf("%d:%d:%d-%d:%d:%d", s.Start.Line, s.Start.Col, s.Start.Offset, s.End.Line, s.End.Col, s.End.Offset)
}

// dumpQuote quotes s byte by byte: printable ASCII stays, `"` and `\` are
// escaped, everything else is \xHH (lowercase).
func dumpQuote(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c >= 0x20 && c < 0x7f:
			b.WriteByte(c)
		default:
			b.WriteString(`\x`)
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// dumpKindNames are the Go constant names of token kinds, which are also
// the variant names of the Kind enum in selfhost/lexer (EOF is Eof there,
// because enum variants are UpperCamel, so it is Eof here too).
var dumpKindNames = map[token.Kind]string{
	token.Illegal: "Illegal", token.EOF: "Eof", token.Newline: "Newline",
	token.Ident: "Ident", token.Int: "Int", token.Float: "Float", token.Duration: "Duration",
	token.Rune: "Rune", token.String: "String", token.RawString: "RawString", token.Bytes: "Bytes",
	token.ImportPath: "ImportPath", token.Hole: "Hole",
	token.Plus: "Plus", token.Minus: "Minus", token.Star: "Star", token.Slash: "Slash",
	token.Percent: "Percent", token.Shl: "Shl", token.Shr: "Shr", token.Amp: "Amp",
	token.Pipe: "Pipe", token.Caret: "Caret", token.Tilde: "Tilde", token.Bang: "Bang",
	token.AndAnd: "AndAnd", token.OrOr: "OrOr", token.Coalesce: "Coalesce", token.Question: "Question",
	token.Eq: "Eq", token.NotEq: "NotEq", token.Lt: "Lt", token.LtEq: "LtEq", token.Gt: "Gt", token.GtEq: "GtEq",
	token.Assign: "Assign", token.PlusAssign: "PlusAssign", token.MinusAssign: "MinusAssign",
	token.StarAssign: "StarAssign", token.SlashAssign: "SlashAssign", token.PercentAssign: "PercentAssign",
	token.AmpAssign: "AmpAssign", token.PipeAssign: "PipeAssign", token.CaretAssign: "CaretAssign",
	token.ShlAssign: "ShlAssign", token.ShrAssign: "ShrAssign",
	token.Arrow: "Arrow", token.FatArrow: "FatArrow", token.DotDot: "DotDot", token.DotDotEq: "DotDotEq",
	token.Dot: "Dot", token.Comma: "Comma", token.Colon: "Colon", token.LParen: "LParen",
	token.RParen: "RParen", token.LBrack: "LBrack", token.RBrack: "RBrack", token.LBrace: "LBrace",
	token.RBrace: "RBrace", token.HashLBrace: "HashLBrace",
	token.As: "As", token.Break: "Break", token.Const: "Const", token.Continue: "Continue",
	token.Defer: "Defer", token.Else: "Else", token.Ensures: "Ensures", token.Enum: "Enum",
	token.Fn: "Fn", token.For: "For", token.If: "If", token.In: "In", token.Interface: "Interface",
	token.Invariant: "Invariant", token.Let: "Let", token.Match: "Match", token.Pub: "Pub",
	token.Requires: "Requires", token.Return: "Return", token.Scope: "Scope", token.Select: "Select",
	token.Struct: "Struct", token.Test: "Test", token.Type: "Type", token.Use: "Use",
	token.Uses: "Uses", token.Var: "Var", token.While: "While",
}

// DumpKindName is the name Dump prints for k.
func DumpKindName(k token.Kind) string {
	if n, ok := dumpKindNames[k]; ok {
		return n
	}
	return k.String()
}
