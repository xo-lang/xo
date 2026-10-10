package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
)

// Dump parses src and prints the canonical syntax tree dump shared with the
// self hosted parser in selfhost/parser (decision 0127): a `== <file>`
// line, the tree in the form of ast.Text, and one
// `diag <code> <line>:<col>-<line>:<col> "<message>"` line per diagnostic
// in sorted order. `xo astdump` prints it. internal/selfhost requires the
// Xo parser to print the same text for every file both parsers accept,
// and to report diagnostics for exactly the files this parser rejects.
func Dump(file string, src []byte, mode Mode) string {
	f, ds := ParseFile(file, src, mode)
	var b strings.Builder
	b.WriteString("== " + file + "\n")
	b.WriteString(ast.Text(f))
	for _, d := range ds {
		fmt.Fprintf(&b, "diag %s %d:%d-%d:%d %s\n", d.Code, d.Span.Line, d.Span.Col, d.Span.EndLine, d.Span.EndCol, strconv.Quote(d.Message))
	}
	return b.String()
}
