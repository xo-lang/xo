# A closure's tail `if` or `match` took the body want's Result type

Status: Observed (2026-11-11, the scarlet session; fixed by Xo Lang 1)

**Symptom.** `let get = fn(d: Int) { if d > 0 { d } else { 0 } }` was
XO1201 "converting `Result[Int, _]` to `Int`" on `xo check` and native
builds, at file "" line 0. The Go backend printed the right answer. A
`-> Int` on the closure, or a body that did not branch, built.

**Cause.** A closure with no written result or error is checked against
`ret_want(result, err_var)` (compiler/check expr.xo fn_lit), so that a
`?` in it can infer the error type. Under that want, `if_value` and
`match_expr` give an `if` or `match` the type `Result[T, err_var]`. When
nothing in the body fails, `err_var` stays unbound and the closure's type
is `fn(..) -> T`, but the recorded type of its tail `if` stayed the
Result, which native lowering cannot turn into T. The closure's body is
lowered as a queued job after the enclosing expression, so `b.at` was the
empty position when ret_at's conversion recorded the XO1201.

**Fix.** fn_lit retypes the tail `if`/`match`/block expressions that are
`Result[_, err_var]` as the plain result when `err_var` is unbound
(untry_tails). ret_at records at the returned expression, and record_err
falls back to the closure literal or the function being built. `xo dev
errorcheck` fails any diagnostic without a file and line. Golden
closure_branch_result.

**Addendum (2026-11-12): an empty position was a signal.** The fallback
broke cmd/xo TestUseGoNoGCTypesInOneError: compiler/ssa nogc.xo groups
`use go` records by whether they have a file, a record with none being a
package's type (listed in its package's XO1201) and one with a file a use
(counted). With the fallback every type record had a file, so the type
was counted as a use and the list vanished. Fix (e11226596):
BuildErr.borrowed marks a fallback position, the grouping treats it as
no position, and the once-per-position key is the record's own position
again. Before giving a missing value a default, find who reads "missing"
as information (grep the field's `.is_empty()` and `== 0` readers).
