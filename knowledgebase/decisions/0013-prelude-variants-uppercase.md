# 0013: Ok, Err, Some, None; implicit Some

Status: Accepted (2026-10-07)

## Context
Lowercase `ok err some none` contradicted the naming rule and, being
unshadowable, took the names `err` and `result` from user code. Agents
reported reaching for `Some(x)` repeatedly. Wrapping every optional
assignment in `some(...)` was listed as friction. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
- Variants are `Ok Err Some None`, usable without a prefix.
- `result` and `old` are contextual inside `ensures` only.
- A `T` is accepted where `T?` is expected (wrapped in `Some`). This is the
  second implicit conversion, alongside error conversion at `?`.

## Consequences
- Matches Rust and Swift priors. Frees `err` and `result` as variable names.
- Implicit `Some` slightly weakens "no implicit conversions"; it cannot
  change meaning because `T` and `T?` never overlap.
