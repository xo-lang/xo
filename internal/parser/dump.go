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
// in sorted order, each followed by its notes (`  note <span> "<message>"`,
// `-` for no span) and fixes (`  fix "<title>"`, then `    edit <span>
// "<replacement>"` per edit; decision 0141, stage 7). `xo astdump` prints it. internal/selfhost requires the
// Xo parser to print the same text for every file both parsers accept,
// and to report diagnostics for exactly the files this parser rejects.
func Dump(file string, src []byte, mode Mode) string {
	f, ds := ParseFile(file, src, mode)
	var b strings.Builder
	b.WriteString("== " + file + "\n")
	b.WriteString(ast.Text(f))
	for _, d := range ds {
		fmt.Fprintf(&b, "diag %s %d:%d-%d:%d %s\n", d.Code, d.Span.Line, d.Span.Col, d.Span.EndLine, d.Span.EndCol, strconv.Quote(d.Message))
		for _, n := range d.Notes {
			sp := "-"
			if n.Span != nil {
				sp = fmt.Sprintf("%d:%d-%d:%d", n.Span.Line, n.Span.Col, n.Span.EndLine, n.Span.EndCol)
			}
			fmt.Fprintf(&b, "  note %s %s\n", sp, strconv.Quote(n.Message))
		}
		for _, f := range d.Fixes {
			fmt.Fprintf(&b, "  fix %s\n", strconv.Quote(f.Title))
			for _, e := range f.Edits {
				fmt.Fprintf(&b, "    edit %d:%d-%d:%d %s\n", e.Span.Line, e.Span.Col, e.Span.EndLine, e.Span.EndCol, strconv.Quote(e.Replace))
			}
		}
	}
	return b.String()
}
