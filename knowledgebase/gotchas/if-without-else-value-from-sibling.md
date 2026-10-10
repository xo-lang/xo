# An open `else if` chain at a closure tail fails on the last `if`

Status: Observed (2026-10-10, Codex r4 high, task 01); help added the
same day.

Symptom: a closure ending with
`if a { n.add(1) } else if b { m.add(1) }` (no final `else`) failed
with `XO0401 expected Int, found ()` on the inner `if`, with no note or
fix. The program reads as a statement; nothing in it asks for an `Int`.

Cause: the closure's result type is inferred from its tail. The first
branch's `n.add(1)` returns `Int` (Atomic.add returns the new value), so
the result became `Int`; the `else if` without `else` has type `()`
(core 3.5) and cannot give one. The expected type came from a sibling
branch, which the bare message did not say.

Fix: `internal/check/ifnoelse.go` adds a note naming the missing `else`
to every such XO0401, and when the expected type was still open before
the chain was checked (it came from a branch), a second note and a
machine fix that put `let _ =` before each branch value (the discard
form of core 2.6). A branch whose value is a `Result` gets no fix,
since `let _ =` on a Result is XO0206. Gotcha inside the fix: the tail
of a closure that may fail is recorded as `Result[T, _]` with an open
error variable, so the check unwraps that before deciding a branch is a
real Result.
