# 0027: Spec 0.3 clarifications

Status: Accepted (2026-10-07)

## Context
Remaining 0.2 paper test gaps that only needed a ruling. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
- `uses (a, b)` allowed on declarations; parentheses optional there.
- Three implicit conversions, listed in core 2.10: `Some` wrapping, interface
  conversion, error conversion at `?`.
- A block ending in a statement is `()`; `while { }` without `break` is `Never`.
- Generic struct literals and enum variants infer type arguments; qualified
  variants (`http.ClientErr.Canceled`) in patterns; `UpperCamel` in a pattern
  is always a variant.
- Scalars, `Str`, `Duration`, `Time`, std errors are `Display`; `Float` is
  `Ord` with NaN last; `Duration` may be negative.
- Pure means no capability effects; `Mutex`, `Atomic`, `Chan` are allowed.
- `Router`, `Next`, `call`, `serve` carry no effects; registration accounts for them.
- Interface satisfaction ignores `pub` on methods; std struct fields are `pub`.
- `expect` and `testing` available in all `_test.xo` files; FakeClock ordering,
  MemFs mtimes, and `fail_next` matching specified.
