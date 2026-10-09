# Parser lookahead past the end of the tokens panics

Status: Observed (2026-10-09)

Symptom: `FuzzParse` panicked with `index out of range` on inputs such as
`fn{{le#` and `fn{for:=range {`, inside the habit recovery code
(internal/parser/habits_*.go) added for decision 0087.

Cause: habit detection looks several tokens ahead (`p.toks[i+2]`) to
recognize a foreign form, and short or truncated inputs end before that.
`p.peek(n)` was already safe; direct indexing was not.

Fix: habit code reads tokens through `p.tokAt(i)`, which returns the EOF
token past the end. When adding lookahead anywhere in the parser, use
`peek` or `tokAt`, and add the new form's input to the fuzz seeds
(`FuzzParse` seeds itself from internal/fix/testdata/habits and
bench/habits/drafts), then run
`go test -run '^$' -fuzz=FuzzParse -fuzztime=300s ./internal/parser/`.
