# `ssa.Type.Heap` called scalar structs heap types

Status: Observed (2026-10-08, `internal/ssa`, found by the RC pass goldens)

Symptom: after `InsertRC`, `fn less(a: Rec, b: Rec)` with
`struct Rec { key: Int, id: Int, weight: Float }` printed its
parameters as `borrow`, so the RC pass treated a struct of numbers as a
heap value.

Cause: `Heap` guards against recursive types with a `seen` set and
answers "heap" when it meets a type again (a recursive struct or enum
is boxed). The set was never cleared, so the second `Int` field counted
as a recursion and made `Rec` a heap type.

Fix: `seen` is the path of types being visited (removed again on
return), not every type visited. Any guard against recursion in a type
walk needs path semantics; a visited set is only right for a yes/no
reachability question whose answer does not depend on revisiting.
