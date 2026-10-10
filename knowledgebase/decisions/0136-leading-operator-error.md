# 0136: A line that starts with a binary operator is XO0178

Status: Accepted, implemented (2026-10-10). Spec: core 1.3, 10.6
(`XO0178`). Code: `internal/parser/leading_op.go` (called from the token
pass in `internal/parser/habits_lex.go`), `internal/diag` (code,
description, `xo explain`). Tests: `internal/fix/testdata/habits/codex-leading-or`
and `codex-leading-plus` (one diagnostic each, fixed program checks),
`internal/check/r4habits_test.go`, `internal/parser` error recovery.

## Context

Codex r4 (bench/results/codex-xo-r4-2026-10-10.md) continued an `if`
condition on the next line with a leading `||`:

```
if job.priority > best.priority
  || (job.priority == best.priority && job.sequence < best.sequence) {
```

Xo ends a statement at a newline after an identifier, literal, `)`, `]`,
`}`, or `?` (Go's rule, core 1.3), so the condition ended at the first
line. The parser reported `XO0160 expected {`, then took `|| (...)` for
a Rust closure with no parameters and offered the XO0177 fix
`fn() { ... }`, which `xo fix` would apply blindly and which changes the
meaning, then `XO0175` for the `{`. Three errors, one of them a wrong
machine fix, for one habit (Rust, Swift, Kotlin, and some Go style guides
break lines before the operator).

## Decision

- A line whose first token is a binary operator that cannot start an
  expression (`||`, `&&`, `??`, `|`, `^`, `+`, `/`, `%`, `<<`, `>>`,
  `==`, `!=`, `<`, `<=`, `>`, `>=`, and `*` or `&` followed by a space),
  after a line that ended a statement, is error `XO0178`: "a line cannot
  start with the binary operator `||`; end the previous line with it".
- The machine fix moves the operator to the end of the previous line.
  The check runs on the token stream before parsing (like the other
  lexical habits of decision 0087) and drops the newline, so the parser
  reads the expression as continued: one diagnostic, no XO0160, XO0175,
  or XO0177 after it, and the checker sees the intended expression.
- It does not fire after `return`, `break`, or `continue` (they end
  their statement), on a line that holds a `=>` (a match arm whose
  pattern is bad keeps XO0165), or for `-`, `!`, `~`, `..`, `..=`
  (they start expressions, so such a line is a valid new statement), nor
  for `*x`, `&x`, or `++` written against their operand (Go and C
  habits with their own diagnostics).
- The language does not change: a leading operator was always an error.
  A new code rather than XO0177, because the habit is about Xo's
  newline rule, not syntax from another language, and its fix is
  mechanical.

## Consequences

- The closure habit (`|x| e`, `|| e`) still fires where an expression is
  expected; it can no longer fire on an operator that continues an
  expression across a newline, since the token pass removes that case
  first.
- The selfhost parser keeps rejecting these files (it reports a generic
  error); the differential compares only accepted files.

## Alternatives

- Accepting a leading operator as a continuation (as the lexer does for
  `.`): two layouts for one expression, `xo fmt` must pick one, and `-`
  and `!` would stay ambiguous, so the rule would have exceptions anyway.
- Fixing only the XO0177 heuristic (no closure after a complete line):
  leaves XO0160 and XO0175 with no help.
