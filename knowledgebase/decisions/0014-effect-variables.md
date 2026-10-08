# 0014: Effect variables, subsumption, parenthesized effect lists

Status: Accepted (2026-10-07). Amended by 0033.

## Context
A reusable worker pool and an HTTP middleware could not be generic over
their callback's effects; one agent added `clock` only so tests could sleep.
`f: fn(T) -> R uses net, clock,` in a parameter list was ambiguous. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- Effect variables in type parameter lists: `[T, fx: Effects]`, used as `uses fx`.
- Subsumption: fewer effects and a convertible (or no) error type are accepted where more are expected.
- In function types, several effects are parenthesized: `uses (net, clock)`.
- `pub` functions with no `uses` are pure (no empty form needed).
- Effect labels have their own namespace.

## Consequences
- The checker needs effect unification (small: sets of labels plus variables).
