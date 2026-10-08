# A std method whose only own type parameter is an effect is a Go method

Status: Observed (2026-10-08, `task.Once.get`, decision 0043)

Symptom: the generated Go fails to build with
`v_o.M_get undefined (type rt.Task_Once[...] has no field or method M_get)`
after the runtime defined `rt.Task_Once_get[T, E](tk, o, f)` by analogy
with `rt.Task_Semaphore_with`.

Cause: `methodIsGoMethod` (internal/gen/decl.go) drops effect parameters
before deciding. `Semaphore.with[R, E, fx]` keeps type parameters, so it is
a function `Task_Semaphore_with[R, E]`; `Once.get[fx]` has none left, so it
is called as the Go method `o.M_get(tk, f)`, with T and E coming from the
receiver `Task_Once[T, E]`.

Fix: count a std method's own non effect type parameters before naming the
runtime function: none means a Go method `M_<name>` on the handle type.
