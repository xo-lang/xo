# A closure body passed for `fn() -> T ! E` is typed `Result[T, Never]`

Status: Observed (2026-10-08, internal/ssa lowering of spawn and
Mutex.with, decision 0068)

Symptom: `s.spawn(fn() { ch.recv() ?? -1 })` failed to build natively
with "the SSA builder does not support converting `Int` to
`Result[Int, Never]`", while `s.spawn(fn() { square(i) })` built.

Cause: the checker checks a closure argument against the parameter type
`fn() -> T ! E` with E inferred as Never and records the expected type
of the body's last expression as `Result[T, Never]` (`Info.TypeOf`).
Expressions whose lowering coerces to their contextual type (`??`, an
`if` or `match` with a value) then tried to wrap a T in that Result,
although a function with error type Never cannot fail and returns a
plain T (`failErr`).

Fix: `fn.typeOf` in internal/ssa/build.go treats `T ! Never`
(`Result[T, Never]`) as T. Any new lowering of a std function that takes
a closure with an error parameter (`task.with_timeout`, `Semaphore.with`,
`Once.get`) will meet the same contextual type.
