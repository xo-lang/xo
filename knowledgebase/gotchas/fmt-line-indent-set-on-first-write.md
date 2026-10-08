# xo fmt: a line takes the indentation in force when its first text is written

Status: Observed (2026-10-08, `internal/format`)

Symptom: `requires cond` followed by `else err` on the next line came out
with `else` at the same depth as `requires` instead of one level deeper,
although the printer raised the indentation before starting the line.

Cause: the printer (`internal/format/format.go`) records a line's
indentation when the first text is written to it (`printer.w`), not when
the line is started (`startLine`). The clause code started the line,
restored the old indentation, and only then wrote `else `.

Fix: write at least the first token of a continuation line before
restoring `p.indent` (see `clause`, `binary`, `armBody`, `postfix`). The
golden case `internal/format/testdata/tricky.xo` covers each continuation
form.
