# xo fmt: a comment inside a list printed on one line moves after it

Status: Observed (2026-10-08, `internal/format`)

Symptom: `g(1, // one` then `2)` on the next line is formatted as `g(1, 2)`
with `// one` on its own line below the statement.

Cause: line comments end their line, and `xo fmt` keeps a bracketed list
on one line unless the source breaks the line right after the opening
bracket. Comments are only placed where a line starts (statements,
members, arms, elements of a split list, chain links), so a comment inside
a joined list waits for the next such place. Nothing is lost, and the
second format is a no op, but the comment no longer sits next to its
element.

Fix: to keep comments next to elements, break the line after the opening
bracket (`g(` newline `1, // one` newline `2,` newline `)`); the list then
stays one element per line.
