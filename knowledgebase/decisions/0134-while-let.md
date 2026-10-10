# 0134: `while let`

Status: Accepted, implemented (2026-10-10). Spec: core 3.5, grammar
(`WhileStmt`). Code: `internal/parser` (parseWhile),
`internal/check/whilelet.go`, `internal/gen` (loopWhile, letPattern),
`internal/ssa/build_whilelet.go` (LLVM and arm64), `internal/format`,
`selfhost/parser` (stmt.xo, ast.xo, dump.xo). Tests:
`internal/llvm/testdata/golden/while_let` (Go, LLVM, and arm64 agree, no
leaks), `internal/check/whilelet_test.go`, parser and formatter cases,
`selfhost/parser/parser_test.xo`.

## Context

Codex r4 (bench/results/codex-xo-r4-2026-10-10.md) wrote
`while let Some(value) = cursor { ... }` in a pagination test, the Rust
and Swift form for "loop while this Option has a value". Xo had `if let`
but no `while let`, so the parser reported `XO0160 expected an
expression, found keyword let` with no help, and the rewrite (`while {
match cursor { Some(v) => ..., None => break } }`) costs an agent a
round trip and a level of nesting. The owner decided on 2026-10-10 to
add the form rather than a habit diagnostic.

## Decision

- `[label:] while let Pattern = Expr [bound N] Block` is a statement.
  The value is computed before each iteration; the body runs with the
  pattern's names bound while the value matches, and the loop ends at
  the first value that does not. Any refutable pattern is allowed, as in
  `if let`.
- `while let v = opt` with `opt` an Option is shorthand for
  `while let Some(v) = opt`, exactly as `if let` (core 2.5).
- `break`, `continue`, labels, and `bound N` work as on `while cond`.
  A real-time function (core 6.5) needs the bound; the XO1402 fix
  inserts it after the value.
- The AST keeps one `WhileStmt` node with `Pattern` and `Value` fields
  (nil Cond), mirroring `IfExpr`. The self hosted parser holds them as
  Lists of zero or one element, like `bound`, so the `Stmt` enum does
  not grow past the arm64 reference counting helper frame
  (gotchas/arm64-rc-helper-frame.md).
- The Go backend's integer range analysis does not analyze a `while
  let` loop (it forgets what the loop assigns and keeps every check in
  it); correctness first, since pattern bindings are rebound each
  iteration.

## Consequences

- One more statement form; no new keyword. `xo fmt` prints
  `while let p = v bound N { }`.
- The checker, Go backend, and SSA lower it like `if let` inside a loop:
  no desugaring in the parser, so `xo astdump` and the formatter see
  the form as written.

## Alternatives

- A habit diagnostic (XO0177) with a rewrite to `while { match ... }`:
  rejected by the owner; the form is common in Rust and Swift, reads
  well, and is consistent with `if let`.
- Desugaring in the parser: the formatter would print the expansion and
  `xo astdump` would no longer match the source.
- `while let` as an expression: a loop that ends when the pattern fails
  has no value, like `while cond`.
