# 0608: BigRat, Go's math/big Rat as an immutable value in std/big

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

Decision 0311 put Go's big.Int in std/big and left Rat and Float for
later. Decision 0602 (accepted 2026-11-11) added BigFloat and left
`Float.Rat` and `SetRat` waiting for a Rat type. The std audit of
2026-11-11 ranks rationals as gap 15 (10 stdlib files).

## Decision

`BigRat` in std/big, shaped like BigInt and BigFloat: an immutable value
with Go's semantics, always in lowest terms with a positive denominator.

- Construction: `rat(a: BigInt, b: BigInt)` (SetFrac),
  `rat_from_int(n: Int)` (SetInt64), `BigInt.to_rat()` (SetInt),
  `rat_from_float(x: Float) -> BigRat?` (SetFloat64: exact; None for NaN
  and the infinities, where Go returns nil), `parse_rat(s: Str) -> BigRat
  ! BigErr` (SetString: `a/b`, decimals with an exponent, `0x` and the
  other prefixes in the numerator; Go reports only `false`, so the error
  text is `big: cannot parse "s" as a rational`).
- Arithmetic: `add`, `sub`, `mul`, `quo`, `neg`, `abs`, `inv`. A zero
  denominator (`rat(a, 0)`, `quo` by zero, `inv` of zero) faults with
  Go's panic text (`division by zero`), as BigInt's `quo` does.
- Queries: `num()` and `denom()` (BigInts), `sign`, `cmp`, `is_int`.
- Conversion: `to_float() -> (Float, Bool)` (Float64 with exact),
  `float_prec() -> (Int, Bool)` (FloatPrec), `text()` (String, `a/b`),
  `rat_text()` (RatString: `a` for an integer), `float_text(prec)`
  (FloatString), `display` as String. BigFloat gains `to_rat() ->
  BigRat?` (Float.Rat; None for an infinity), and `rat.to_big_float(prec,
  mode)` (SetRat).
- The Go backend calls math/big. Native code builds on std/big's BigInt
  limbs: normalization by gcd and Go's algorithms (Knuth's 4.5.1 for
  add and sub, the cross-gcd for mul and quo, quotToFloat64 for
  Float64, the FloatString rounding).
- Tests: a Go-generated corpus (internal/stdtest/testdata/corpus/bigrat)
  of every operation over random and edge values (zero, negative, huge,
  float edges, malformed texts), on go, arm64 and llvm. Also a timing row
  against Go beside it, as every port has.

## Consequences

- Exact fractions (money splits, probabilities, continued fractions)
  need no hand-written gcd code. BigFloat's Rat conversions exist.
- std/big grows by one type, and the spec's section 33 grows by one block.

## Alternatives

- A `Rational` over Int: it overflows silently in exactly the programs
  that want exact arithmetic. Go has no such type either.
- Mutating methods as Go has (`z.Add(x, y)`): BigInt and BigFloat are
  values, and Rat follows them.
