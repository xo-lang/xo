# `testing.run_main(main, os)` with an error free `main` fails to build

Status: Observed (2026-10-08, writing the `test_once` golden program). Resolved 2026-10-08: function values now use their natural type (`internal/gen/expr.go`, `call.go`), so the function adapter is applied once; golden `test_run_main_void`.

Symptom: a test module whose `fn main(os: Os)` has no `!` and a test that
calls `testing.run_main(main, os)` fails the Go build:
`cannot use r.E (variable of interface type rt.IError) as rt.Never value in
argument to rt.Absurd[rt.IError]`. The same failure occurs on the commit
before decision 0043, so it is not caused by it.

Cause (suspected): the function value adapter for `main` coerces the
error of a `Result[Unit, IError]` as if its static error type were
`Never` (`coerce` in internal/gen/expr.go emits `rt.Absurd`), so `main` is
emitted with an `IError` result while the checker typed it as error free.

Workaround: declare `fn main(os: Os) ! { ... }` in modules tested through
`run_main` (test_once, test_map_until, test_signal_after do).

Next step: reproduce with a two line module
(`fn main(os: Os) {}` plus a `run_main` test), dump the generated Go with
`XO_BUILD_DIR` set, and make `funcValue`/`adaptFunc` use main's emitted
signature (error `IError`) instead of its checked one.
