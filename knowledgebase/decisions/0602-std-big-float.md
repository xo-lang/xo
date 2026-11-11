# 0602: big.BigFloat, arbitrary precision floats (Go's math/big Float)

Status: Accepted (owner, 2026-11-11 morning, via the lead; proposed by Xo Lang 3, 2026-11-11). Accepted as proposed.

## Context

Decision 0311 put Go's big.Int in std/big and left Rat and Float as later
stages, "each with its own ADR when a user needs them". The std audit of
Go programs (Xo Lang 3, 2026-11-11, gap 7) found math/big.Float in 25
programs, 30 uses: Float, SetPrec, SetMode, Int64, Int, IsInf, MantExp,
Text, Append, Format, Rat and SetRat. Rat is gap 23 and has no Xo type yet.

## Decision

1. **`big.BigFloat`, Go's big.Float as an immutable value**, in the style
   of BigInt (decision 0311): every operation returns a new value. A value
   carries Go's precision (bits, 0 to 2^32 - 1), rounding mode and
   accuracy, and is zero, finite or infinite with a sign (-0 and -Inf
   exist; NaN does not: the operations Go panics on with ErrNaN fault
   with ErrNaN's text). A finite value is `mant` (a BigInt with no
   trailing zero bits) times 2^(exp - mant.bit_len()), so `exp` is Go's
   exponent and `mant.bit_len()` is MinPrec.

2. **Results take Go's `new(Float)` precision and the receiver's mode**:
   `x.add(y)` is Go's `new(Float).SetMode(x.Mode()).Add(x, y)`, so the
   result has precision max(x.prec, y.prec). Every arithmetic method
   takes `prec: Int = 0` to round to another precision instead (Go's
   `new(Float).SetPrec(p).SetMode(x.Mode()).Add(x, y)`). `with_prec` and
   `with_mode` are SetPrec and SetMode.

3. **The surface** (spec/std.md section 33):
   - `from_float(x: Float, prec: Int = 0)` (NewFloat, SetFloat64; 53
     bits by default), `float_from_int(n: Int, prec: Int = 0)` (SetInt64,
     64 bits), `BigInt.to_big_float(prec: Int = 0)` (SetInt, max(bit
     length, 64) bits), `inf(neg: Bool)`, `parse_float(s, base: Int = 0,
     prec: Int = 0, mode: RoundingMode = ToNearestEven) -> BigFloat !
     BigErr` (ParseFloat, Go's error texts).
   - `add`, `sub`, `mul`, `quo`, `sqrt`, `neg`, `abs`, `cmp`,
     `mul_pow2(e)` (SetMantExp), `mant_exp() -> (BigFloat, Int)`.
   - `prec`, `min_prec`, `mode`, `acc`, `sign`, `signbit`, `is_inf`,
     `is_int`.
   - `to_int() -> (Int, Accuracy)` (Int64), `to_float() -> (Float,
     Accuracy)` (Float64), `to_big() -> (BigInt?, Accuracy)` (Int; None
     for an infinity).
   - `text(format: Int, prec: Int) -> Str` with Go's formats ('e', 'E',
     'f', 'g', 'G', 'b', 'p', 'x', shortest when prec is negative);
     `display()` is Go's String(), `text('g', 10)`.
   - enums `RoundingMode` and `Accuracy` in Go's order with Go's names.

4. **Go's algorithms where the result depends on them**: rounding
   (float.go round), Sqrt's Newton iteration on 1/sqrt(x), parsing's
   powers of five at prec + 64 bits, and the decimal conversion and
   shortest formatting of ftoa.go and decimal.go; Add, Sub, Mul and Quo
   round the exact result once, which is what Go's do. The native code
   works on std/native/big.xo's limbs; the Go backend's twin calls
   math/big. A Go generated corpus (scripts/stdcorpus bigfloat) checks
   every operation, conversion and format against Go on every backend.

## Consequences

- Programs that need more than 53 bits, or a rounding mode, get Go's
  results bit for bit.
- Immutable values allocate on every operation, as BigInt's do; Sqrt
  and parsing at thousands of bits are slower than Go's in-place forms.
- `Format` (fmt.Formatter) has no Xo counterpart: `text` covers its
  verbs. `Append` is `text` on a builder. Rat, SetRat and Float.Rat wait
  for big.Rat (gap 23).

## Alternatives

- A mutable Float with Go's `z.Add(x, y)`: rejected for the reasons
  0311 gives for Int (aliasing; Xo's values are immutable).
- Results at the receiver's precision instead of max(x, y): simpler to
  state, but not what `new(Float).Add` does, so ported code would round
  differently.
- Wrapping Go's math/big on every backend: there is no Go at run time
  on the native backends.
