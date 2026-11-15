# A Mutex.with closure ending in if / else if let without else is XO0540

Status: Observed (2026-11-15, std/sql batch S1)

## Symptom

A closure passed to `Mutex.with` whose body ends in `if a { ... } else if
let Some(x) = y { ... }` with no final `else`, and whose declared result
is `() ! E`, is refused with XO0540 (a value is missing on a path).

## Cause

Not traced to the checker's code yet (the batch S1 session fixed it and
moved on): the chain without a final `else` does not count as the
closure's `() ! E` result. Anticipated: any closure with a declared
Result type ending this way.

## Fix

End the closure with an explicit `Ok(())` after the chain (or add the
`else`).
