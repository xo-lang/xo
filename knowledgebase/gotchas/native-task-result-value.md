# A task whose value is a Result failed its scope natively

Status: Observed (2026-10-09, while writing `task.map_concurrent` in
std/native/task.xo)

Symptom: on the native backends, a scope that spawns `fn() -> Result[Int,
Str] { half(x) }` and awaits every task returned the scope's zero value
(an empty list, 0) as soon as one task's value was an `Err`; the Go
backend returned every Result. No fault, no error, no diagnostic.

Cause: the task runner (`runner` in internal/ssa/build_conc.go) decided
whether a task failed from its closure's SSA result type alone: an enum
with `Ok` and `Err` variants. A closure that cannot fail but returns a
Result value has the same SSA result type as one that fails, so an `Err`
value was recorded as the task's failure, which fails the scope (core
7.1) and unwinds the body.

Fix: `runner` takes whether the closure can fail (its checker error
type), and only then reads the tag. Golden
`internal/llvm/testdata/golden/task_native` (`results`). Anything else
that inspects a lowered Result to decide failure must use the checker's
error type, never the SSA shape.
