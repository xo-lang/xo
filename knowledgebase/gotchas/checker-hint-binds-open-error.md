# Unifying a call's result with the expected type too early

Status: Observed (2026-10-07, building the P1 checker)

Symptom: `s.spawn(fn() { permits.with(fn() { ... }) })` and
`m.update(k, init, fn(var b) { ... })` inside a closure reported
"expected `Result[(), Never]`, found `()`" and XO0203.

Cause: before checking arguments, the checker unifies a generic call's
result with the expected type to help inference. For `with[R, E, fx]` the
result seen by the caller is `Result[R, E]` while `E` is still an open
variable. The expected type was the enclosing closure's own open result
variable, which got bound to `Result[R, E]`; `E` then defaulted to `Never`,
leaving the closure typed as returning `Result[(), Never]`.

Fix (`callFunc` in `internal/check/call.go`): skip the early unification when
the expected type is itself an open variable, and when the error type still
has open variables unify only the success type. A `Result[T, Never]` should
never appear in a message; if one does, look for a binding made before the
error variable was decided.
