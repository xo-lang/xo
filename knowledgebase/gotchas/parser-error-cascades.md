# Parser error cascades and double reports

Status: Observed (2026-10-07, building the P1 parser)

Symptoms seen while building `internal/parser`:

- An invalid character (`@`) was reported twice: once by the lexer (XO0150)
  and again by the parser as "expected an expression, found illegal".
- After a hard error inside a statement, continuing to parse the same
  statement produced follow on errors.

Fixes in place:

- The parser never reports an error located at an `Illegal` token; the lexer
  already did.
- A hard error sets a per statement flag that suppresses further errors until
  the next statement, member, match arm, or declaration boundary, then the
  parser skips to the next newline or closing brace at the same depth
  (`syncStmt`) or to the next column one declaration keyword (`syncTop`).
  Nested blocks save and restore the flag, so an error inside a closure body
  is recovered inside that body and the enclosing statement continues.
- Missing commas before a newline in a list, `else` on a new line, and comma
  separated match arms are soft errors: reported with a fix, then parsing
  continues as if the source were correct.

When adding grammar, make every loop either consume a token or stop
(`if p.pos == before { break }`), and add an error recovery case to
`TestErrorRecovery`.
