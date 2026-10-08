# 0016: Lost update check and Map.update; Mutex.with semantics

Status: Accepted (2026-10-07). Amended by 0023 (narrower XO0520).

## Context
Under value semantics, get, modify, put is easy to get wrong silently
(gotcha value-semantics-lost-update). `Mutex.with` semantics were guessed by
3 of 5 agents. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- XO0520: a local `var` mutated and never read afterwards is an error.
- `Map.update(k, init, fn(var V) -> R) -> R`.
- `Mutex.with[R, E, fx]` returns the closure's value or error; writes back only on success.
- `var` methods may be called on fields of `var` bindings and `var` parameters.

## Consequences
- XO0520 does not catch every lost update (a later read of the stale copy hides it). `update` is the real fix.

## Evidence
- Harder tasks 2026-10-07: rollback on Err forces two critical sections (06). ([results](../../bench/results/planted-and-harder-2026-10-07.md))
