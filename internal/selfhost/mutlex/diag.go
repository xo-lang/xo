package mutlex

// Code names a lexer error (the codes of internal/diag the lexer uses;
// the mutation generators only count them).
type Code string

const (
	EmptyInterp         Code = "XO0157"
	InvalidChar         Code = "XO0150"
	InvalidDuration     Code = "XO0155"
	InvalidEscape       Code = "XO0153"
	InvalidRune         Code = "XO0152"
	MalformedImportPath Code = "XO0158"
	MalformedNumber     Code = "XO0154"
	UnterminatedInterp  Code = "XO0156"
	UnterminatedString  Code = "XO0151"
)

// List counts the errors of a lex.
type List struct{ N int }

// Errorf counts one error.
func (l *List) Errorf(code Code, sp Span, format string, args ...any) { l.N++ }
