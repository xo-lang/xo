# A Go composite literal reads a field after a call in the same literal

Status: Observed (2026-10-10)

Symptom: the self hosted parser's tree for Kotlin's `val x = 1` (a habit,
decision 0087) differed from the Go parser's in one span: the Go parser
gave the recovered `BindPat` the span of the `=` after the name, not of
the name.

Cause: internal/parser builds the pattern as
`&ast.BindPat{Base: ast.Base{Sp: p.tok.Span}, Name: p.expectIdent("a name")}`.
Go evaluates the calls of an expression in order, but the order of a
plain field read (`p.tok.Span`) relative to them is unspecified; gc reads
it after `expectIdent` has advanced `p.tok`.

Fix: a port of Go code must take such a read after the call (habits_stmt.xo
reads `p.tok.span` after `expect_ident`). When porting a composite literal
that mixes reads of parser state with calls that change it, check the Go
dump of one input before trusting the reading order of the source. The
Go parser's span is kept (it is what `xo fix` and the checker see).
