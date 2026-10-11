# `return b.freeze()` inside a loop counts as a move for the next iteration

Status: Observed (2026-10-11, porting std/toml to Xo)

Symptom: XO0510 "use of moved value `b`" on every use of a
`BytesBuilder` in a `while` loop whose body ends one branch with
`return Ok(b.freeze().to_str() ?? "")`, and on passing the builder to a
helper declared `fn f(var b: BytesBuilder)`.

Cause: the move checker (internal/check/flow.go) walks a loop body twice
and does not treat `return` as ending the path, so the `freeze` of one
iteration is seen as having moved `b` for the next. Passing a builder as
an argument moves it (core 5.3), also to a `var` parameter.

Fix (in the code that hit it): `break` out of the loop and freeze after
it; have helpers return the text they produce (`b.write_str(p.escape()?)`)
instead of taking the builder. Next experiment for the checker: end the
path at `return` in flow.go's loop walk, with a rules_test.go case
`while { if c { return b.freeze() } b.write(...) }`.
