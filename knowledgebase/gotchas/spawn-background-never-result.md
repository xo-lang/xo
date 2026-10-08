# `s.spawn_background(fn() { while { ... } })` does not type check

Status: Observed (2026-10-07, writing the `scope_tasks` golden test)

Symptom: XO0402 "cannot infer type argument `T` of
`Scope.spawn_background`".

Cause: the prelude declares `Scope.spawn_background[T, E, fx]` returning
`Task[T, E]`; a closure whose body is an infinite loop has type Never, so
nothing determines T. (`task.Group.spawn_background` takes `fn() -> () ! E`
and has no T.)

Workaround: give the loop a condition, or end the closure with `()`. A
checker or prelude change (T defaults to `()` when the closure is Never)
would remove the trap; the checker is not changed by the code generator
work.
