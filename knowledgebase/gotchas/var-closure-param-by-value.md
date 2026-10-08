# A `var` closure parameter for a by value function type was accepted

Status: Observed (2026-10-08, while adding `Mutex.with_read`)

Symptom: `m.with_read(fn(var s) { s.put("k", 1) })` passed `xo check`,
and `xo run` then failed with a code generator bug: `cannot use a0
(variable of struct type rt.Map[string, int64]) as *rt.Map[string, int64]
value in argument to x_t1`. The same held for any parameter of type
`fn(T)` given a closure `fn(var x) ...`.

Cause: `fnLit` (internal/check/expr.go) built the closure's type with
`Var` set when either the expected type or the literal said `var`, and
`funcAssignable` ignores `Var`, so `fn(var T)` was accepted for `fn(T)`.
The Go backend passes `var` parameters by pointer, so the call did not
type check in Go. Even if it had, the changes would only have reached a
copy: for `with_read` that is a lost update dressed as a write.

Fix: a `var` closure parameter where the expected function type takes the
value by value is error XO0401 with a note pointing at `with` (decision
0063). The binding stays mutable inside the closure so its uses do not
cascade into XO0501 errors. The other direction (`fn(x)` for `fn(var T)`,
a read only view) is unchanged. Tests: `TestWithRead` in
internal/check/rules06_test.go.
