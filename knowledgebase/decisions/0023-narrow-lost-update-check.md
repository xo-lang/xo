# 0023: XO0520 applies only to values copied out of collections

Status: Accepted (2026-10-07). Amends 0016. Amended by 0034.

## Context
XO0520 as written in 0.2 was unclear for `var` closure parameters and reads
in `defer`, and fired on a test whose last use of a `var` was a mutating call
made for its return value. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
XO0520 applies only to a local `var` initialized by copying an element out of
a collection (`get`, index, `first`, `last`, `find`, optionally with `??` or
`?`). Reads in `defer` count. Parameters are never checked.

## Evidence
- Planted bugs 2026-10-07: the narrowed XO0520 misses a local copy mutated after append (02 X6). ([results](../../bench/results/planted-and-harder-2026-10-07.md))
