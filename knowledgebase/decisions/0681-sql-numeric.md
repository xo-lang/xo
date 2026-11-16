# 0681: Postgres numeric in std/sql as sql.Numeric (task #113)

Status: Proposed (Xo Lang 3, 2026-11-16). Decision 0679 left numeric's
representation to its own decision before it lands.

## Context

A Postgres `numeric` column (OID 1700) reads as `Text` today: exact, but
a program must parse it itself, and `Row.float` refuses it. pgx v5 reads
it into `pgtype.Numeric`: a `*big.Int` coefficient and an `int32`
exponent (the value is `Int * 10^Exp`), with `NaN` and an
`InfinityModifier` (Postgres 14 added `Infinity` and `-Infinity`), so
the value and its scale survive: `1.50` is coefficient 150, exponent -2,
not `1.5`. 22 map.txt rows wait on this (pgtype.Numeric, its codec and
scanners, NumericOID), and `range_num` (numrange, nummultirange, 4 more
rows) waits on the element type.

std/big has `BigInt` (decision 0311) and `BigRat` (decision 0608), both
on every backend (std/rt/std_gen_big.go).

## Decision

1. `sql.Numeric`, pgx's shape:

   ```
   enum Numeric {
     Finite(coef: big.BigInt, exp: Int)
     NaN
     Infinity
     NegInfinity
   }
   fn (n: Numeric) display() -> Str          // Postgres's text: 1.50, -0.003, NaN, Infinity
   fn (n: Numeric) to_rat() -> big.BigRat?   // exact; None for NaN and the infinities
   fn (n: Numeric) to_float() -> Float?      // nearest Float (pgx's Float64Value)
   fn (n: Numeric) to_int() -> Int?          // when integral and in range (pgx's Int64Value)
   fn parse_numeric(s: Str) -> Numeric ! SqlErr   // Postgres's text, and 1e-3 forms (pgx's ScanScientific)
   ```

   `Finite` keeps the scale the server sent: the text `1.50` reads as
   coefficient 150, exponent -2, and displays as `1.50` again. Equality
   is structural (derive Eq), so `1.50` and `1.5` differ, as pgx's
   Numeric values do; `to_rat` compares by value.
2. Readers `Row.numeric(name) -> Numeric` and `numeric_opt`, and the
   parameter `sql.numeric(n)` (its display text). `Row.float` keeps
   refusing a numeric column: a lossy read is `numeric(c)?.to_float()`.
3. `Row.range_num`, `multirange_num` and `sql.range_num` use
   `Range[Numeric]` (decision 0679's range types).
4. The parsing and formatting follow pgx v5.12.0 pgtype/numeric.go
   (parseNumericString, the scientific form, encodeText), with its
   tests ported, and pg-diff TYPED cases against pgx on Postgres 14 and
   17 (scales, negative exponents, NaN, Infinity, 1000-digit values).

## Consequences

- A numeric column is read without loss and without a decimal type of
  its own in std: arithmetic goes through `to_rat` (exact) or `to_float`
  (fast, lossy). Money code keeps its scale through a round trip.
- std/sql gains a dependency on std/big (pure Xo, already on every
  backend); a program that reads no numeric pays nothing at run time.
- The existing `Text` reading stays: `Row.str` on a numeric column still
  returns its text, so no program changes.

## Alternatives

- `big.BigRat` as the reader's type: exact arithmetic directly, but it
  drops the scale (`1.50` reads back as `3/2`) and has no NaN or
  infinity; the round trip would not be pgx's.
- A decimal type in std (`std/decimal`, shopspring/decimal style):
  arithmetic with scale rules, but a new module and API to design and
  prove for one driver's sake; can come later and convert from
  `Numeric`.
- `Float`: lossy for money and for values past 15 digits; pgx offers it
  only as a conversion, as `to_float` does here.
- Keep `Text`: the status quo, which leaves the 22 rows and numeric
  ranges as gaps.
