# 0011: Explicit failure with Err(e) and a Never type

Status: Accepted (2026-10-07)

## Context
All 5 paper test agents had to guess how a `! E` function fails without `?`.
Several also needed a bottom type (`fault`, `proc.exit` in a match arm). (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- In `-> T ! E`, a returned value may be `T` (success), `Err(e)` (failure), or any `Result[T, E]`.
- `T ! Never` is `T`. `Never` types `return`, `break`, `fault`, `proc.exit`.
- `Result.map_err` for inline conversion.

## Consequences
- `return Err(e)` is what every agent guessed, so no new habit is needed.
- `T` being a `Result` itself requires explicit `Ok` (XO0203).

## Alternatives
- A `fail e` statement: one more keyword, no agent guessed it.
