// Package token defines the lexical tokens of xo and their source positions.
package token

import "fmt"

// Kind is the kind of a token.
type Kind int

// Token kinds. The order groups literals, operators, and keywords so that
// range checks such as IsKeyword stay simple.
const (
	Illegal Kind = iota
	EOF
	Newline // statement terminator inserted by the lexer (core 1.3), or ';'

	literalBeg
	Ident
	Int
	Float
	Duration
	Rune
	String // plain or triple quoted string, possibly interpolated
	RawString
	Bytes
	ImportPath // path after `use`, such as std/http
	Hole       // ???
	literalEnd

	operatorBeg
	Plus     // +
	Minus    // -
	Star     // *
	Slash    // /
	Percent  // %
	Shl      // <<
	Shr      // >>
	Amp      // &
	Pipe     // |
	Caret    // ^
	Tilde    // ~
	Bang     // !
	AndAnd   // &&
	OrOr     // ||
	Coalesce // ??
	Question // ?

	Eq    // ==
	NotEq // !=
	Lt    // <
	LtEq  // <=
	Gt    // >
	GtEq  // >=

	Assign        // =
	PlusAssign    // +=
	MinusAssign   // -=
	StarAssign    // *=
	SlashAssign   // /=
	PercentAssign // %=
	AmpAssign     // &=
	PipeAssign    // |=
	CaretAssign   // ^=
	ShlAssign     // <<=
	ShrAssign     // >>=

	Arrow      // ->
	FatArrow   // =>
	DotDot     // ..
	DotDotEq   // ..=
	Dot        // .
	Comma      // ,
	Colon      // :
	LParen     // (
	RParen     // )
	LBrack     // [
	RBrack     // ]
	LBrace     // {
	RBrace     // }
	HashLBrace // #{
	operatorEnd

	keywordBeg
	As
	Break
	Const
	Continue
	Defer
	Else
	Ensures
	Enum
	Fn
	For
	If
	In
	Interface
	Invariant
	Let
	Match
	Pub
	Requires
	Return
	Scope
	Select
	Struct
	Test
	Type
	Use
	Uses
	Var
	While
	keywordEnd
)

var kindNames = [...]string{
	Illegal:    "illegal",
	EOF:        "end of file",
	Newline:    "newline",
	Ident:      "identifier",
	Int:        "integer literal",
	Float:      "float literal",
	Duration:   "duration literal",
	Rune:       "rune literal",
	String:     "string literal",
	RawString:  "raw string literal",
	Bytes:      "bytes literal",
	ImportPath: "import path",
	Hole:       "???",

	Plus:     "+",
	Minus:    "-",
	Star:     "*",
	Slash:    "/",
	Percent:  "%",
	Shl:      "<<",
	Shr:      ">>",
	Amp:      "&",
	Pipe:     "|",
	Caret:    "^",
	Tilde:    "~",
	Bang:     "!",
	AndAnd:   "&&",
	OrOr:     "||",
	Coalesce: "??",
	Question: "?",

	Eq:    "==",
	NotEq: "!=",
	Lt:    "<",
	LtEq:  "<=",
	Gt:    ">",
	GtEq:  ">=",

	Assign:        "=",
	PlusAssign:    "+=",
	MinusAssign:   "-=",
	StarAssign:    "*=",
	SlashAssign:   "/=",
	PercentAssign: "%=",
	AmpAssign:     "&=",
	PipeAssign:    "|=",
	CaretAssign:   "^=",
	ShlAssign:     "<<=",
	ShrAssign:     ">>=",

	Arrow:      "->",
	FatArrow:   "=>",
	DotDot:     "..",
	DotDotEq:   "..=",
	Dot:        ".",
	Comma:      ",",
	Colon:      ":",
	LParen:     "(",
	RParen:     ")",
	LBrack:     "[",
	RBrack:     "]",
	LBrace:     "{",
	RBrace:     "}",
	HashLBrace: "#{",

	As:        "as",
	Break:     "break",
	Const:     "const",
	Continue:  "continue",
	Defer:     "defer",
	Else:      "else",
	Ensures:   "ensures",
	Enum:      "enum",
	Fn:        "fn",
	For:       "for",
	If:        "if",
	In:        "in",
	Interface: "interface",
	Invariant: "invariant",
	Let:       "let",
	Match:     "match",
	Pub:       "pub",
	Requires:  "requires",
	Return:    "return",
	Scope:     "scope",
	Select:    "select",
	Struct:    "struct",
	Test:      "test",
	Type:      "type",
	Use:       "use",
	Uses:      "uses",
	Var:       "var",
	While:     "while",
}

func (k Kind) String() string {
	if k >= 0 && int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// IsLiteral reports whether k is an identifier or literal kind.
func (k Kind) IsLiteral() bool { return k > literalBeg && k < literalEnd }

// IsOperator reports whether k is an operator or delimiter.
func (k Kind) IsOperator() bool { return k > operatorBeg && k < operatorEnd }

// IsKeyword reports whether k is one of the 28 keywords (core 1.4).
func (k Kind) IsKeyword() bool { return k > keywordBeg && k < keywordEnd }

// IsAssign reports whether k is `=` or a compound assignment operator.
func (k Kind) IsAssign() bool { return k >= Assign && k <= ShrAssign }

var keywords map[string]Kind

func init() {
	keywords = make(map[string]Kind, keywordEnd-keywordBeg)
	for k := keywordBeg + 1; k < keywordEnd; k++ {
		keywords[kindNames[k]] = k
	}
}

// Lookup maps an identifier to its keyword kind, or Ident.
func Lookup(name string) Kind {
	if k, ok := keywords[name]; ok {
		return k
	}
	return Ident
}

// Keywords returns the keyword spellings in declaration order.
func Keywords() []string {
	var out []string
	for k := keywordBeg + 1; k < keywordEnd; k++ {
		out = append(out, kindNames[k])
	}
	return out
}

// Pos is a position in a source file. Line and Col are 1 based; Col counts
// bytes, like Go. Offset is the 0 based byte offset.
type Pos struct {
	Offset int
	Line   int
	Col    int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Span is a half open source range [Start, End) in File.
type Span struct {
	File  string
	Start Pos
	End   Pos
}

func (s Span) String() string {
	return fmt.Sprintf("%s:%d:%d-%d:%d", s.File, s.Start.Line, s.Start.Col, s.End.Line, s.End.Col)
}

// To returns the span from the start of s to the end of e.
func (s Span) To(e Span) Span { return Span{File: s.File, Start: s.Start, End: e.End} }

// StrPart is one piece of a string literal: either cooked literal text or an
// embedded `${expr}` whose raw source is re-lexed by the parser.
type StrPart struct {
	IsExpr bool
	Text   string // cooked text, or raw expression source when IsExpr
	Span   Span   // for expressions: the span of the text between ${ and }
}

// Token is one lexical token.
type Token struct {
	Kind Kind
	Span Span
	Text string // exact source text

	// Value holds the decoded value for Int (decimal string of the value),
	// Rune (the character), RawString, Bytes, and ImportPath tokens.
	Value string
	// Parts holds the parts of a String token. A string without
	// interpolation has exactly one literal part (or none when empty).
	Parts []StrPart
	// Triple is set for """ strings.
	Triple bool
}
