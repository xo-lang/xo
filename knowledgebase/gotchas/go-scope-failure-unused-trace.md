# A scope in a failing closure whose tasks cannot fail does not compile (Go backend)

Status: Observed (2026-10-09), fixed

Symptom: `go build` of the generated code failed with `declared and not
used: x_ftr10` for a `scope` inside a closure of type `fn() -> Int ! ()`
whose spawned tasks cannot fail (golden `fault_tasks`, which isolates
scopes whose tasks only fault).

Cause: `scopeFailure` (internal/gen conc.go) declares the failed task's
error trace and uses it only in the cases of the error types the scope's
tasks can fail with; with no such type the switch has only `default`.

Fix: the trace is marked used (`_ = ftr`) right after it is declared.
