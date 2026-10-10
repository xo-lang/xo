# A leading `||` on a continuation line was read as a Rust closure

Status: Observed (2026-10-10, Codex r4 high, task 06); fixed by decision
0136.

Symptom: an `if` condition continued on the next line with a leading
`||` gave three errors: `XO0160 expected {, found newline`, `XO0177 |x|
body is a Rust closure` with a fix rewriting `|| (...)` to
`fn() { ... }`, and `XO0175`. `xo fix` would have applied the closure
rewrite, which changes the meaning.

Cause: the lexer ends the statement at the newline after `priority`
(Go's newline rule, core 1.3), so the `||` line was parsed as a new
statement, where the closure habit (decision 0087) looks for `||` at
the start of an expression. Habit heuristics that inspect a token at
statement start cannot tell a closure from a continued expression.

Fix: a token pass check before parsing (`internal/parser/leading_op.go`)
reports `XO0178` with a fix moving the operator to the end of the
previous line and drops the newline, so nothing downstream sees the
operator at statement start. When adding a habit that fires on a token
at the start of a statement, check the previous line first: if it ended
a statement with an operand, the token may be a continuation.

Still open: a continuation line starting with `-` or `!` is valid Xo (a
new expression statement), and an unused `Int` or `Bool` expression
statement is accepted silently (checked 2026-10-10:
`a = 1` then `-a` on the next line gives no diagnostic), so
`let x = a` then `  - b` computes `a` and discards `-b`. Next
experiment: a warning for an expression statement with no effect whose
line is more indented than the statement above it, in
`internal/check/stmt.go` (ExprStmt), with a test in
`internal/check/r4habits_test.go`.
