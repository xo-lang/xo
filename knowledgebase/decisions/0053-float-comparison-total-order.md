# 0053: Float comparison: total order for `<`, IEEE for `==`

Status: Accepted (2026-10-08). Implemented in `std/rt/arith.go`
(`FloatCmp`), `std/rt/main.go` (`FNeg`), `internal/gen/expr.go`,
`internal/llvm/fromssa.go` (`floatOrder`), `internal/arm64/isel.go`
(`floatOrder`), `internal/arm64/eq.go`, `internal/ssa/ssa.go`
(`HasFloat`).

## Context
`spec/std.md` made `Float` `Ord` with a total order in which NaN sorts
last, but `spec/core.md` said nothing about what `<` or `==` do on a NaN,
nor about `-0.0`. The backends disagreed:

- The Go backend lowered `<`, `<=`, `>`, `>=` through `rt.FloatCmp`
  (total order, NaN last, `-0.0` equal to `0.0`), `==` as Go `==`
  (IEEE), `min` and `max` through `Cmp` (receiver kept on a tie), and
  `-0.0` as a Go constant expression, which is positive zero.
- The LLVM and arm64 backends lowered `<` to IEEE compares (`fcmp olt`,
  `fcmp` + `b.mi`), false on NaN; `min`/`max` as `a < b ? a : b`, which
  returns the argument on a tie (`(-0.0).min(0.0)` was `0.0`) and drops
  or keeps NaN by operand position; and their box equality helpers
  answered true for the same cell without comparing it, so a recursive
  value holding a NaN equaled itself.

Found writing `internal/arm64/testdata/run/select`
(`gotchas/native-float-nan-order.md`).

## Decision
The Go backend's behavior is the rule; the spec now says it (core 2.1,
std.md 1):

- `==` and `!=` on Float are IEEE: NaN differs from everything, itself
  included; `-0.0 == 0.0`. Structural `==` compares Float fields the same
  way, so an aggregate holding a NaN is not equal to itself, boxed
  (recursive) values included: equality helpers may only short cut on
  the same cell when the type holds no Float (`ssa.Type.HasFloat`).
- `<`, `<=`, `>`, `>=` use the `Ord` total order: `-Inf < ... < -0.0 =
  0.0 < ... < +Inf < NaN`, all NaNs equal in the order.
- `sort`, `sort_by`, derived `Ord`, generic `T: Ord`, `min`, `max`,
  `clamp` use the same order. `a.min(b)` is `b < a ? b : a`, `a.max(b)`
  is `b > a ? b : a` (the receiver on a tie), `a.clamp(lo, hi)` is
  `a.max(lo).min(hi)`.
- Unary `-` flips the sign of every Float: `-0.0` is negative zero (the
  Go backend emits `rt.FNeg`, a call, not a Go constant).

Native lowering:

- LLVM: `a < b` is `and(fcmp ult a, b; fcmp ord a, a)`, `a > b`
  `and(fcmp ugt a, b; fcmp ord b, b)`, `a <= b` `or(fcmp ole a, b; fcmp
  uno b, b)`, `a >= b` `or(fcmp oge a, b; fcmp uno a, a)`.
- arm64: against a non NaN constant one `fcmp` with mi, ls, hi, hs
  (constant on the right) or lt, le, gt, ge (constant on the left);
  otherwise `fcmp x, x; fccmp x, y, #0, vc` with lt (x < y) or ge (x >=
  y), the operands swapped for `>` and `<=`. `min`/`max` reuse it with
  `fcsel`. `selectCompares` accepts the two instruction compare as one
  compare.

## Consequences
- All three backends print the same for every comparison, sort, and
  `min`/`max` over NaN, signed zeros, and infinities
  (`internal/llvm/testdata/golden/float_order`, which also folds 4000
  pseudo random pairs into checksums; `internal/gen/testdata/golden/float_order`
  for `sort`, `sort_by`, `clamp`, derived `Ord`, generics).
- A generic ordering compare of two non constant Floats costs two
  instructions on arm64 instead of one, and LLVM emits two compares and
  a logic op (it folds them when an operand is a known number).
- `a <= b && a >= b` does not imply `a == b`; programs that need IEEE
  `<` can test `is_nan()` first.

## Alternatives
- IEEE `<` everywhere, total order only in `sort` and `Ord` methods
  (what C, Go, and Rust's `<` do): rejected because `T: Ord` generic code
  uses `<`, and the order seen by `largest[T: Ord]` must be the order
  `sort` uses; it would also make `!(a < b)` differ from `a >= b`.
- Total order for `==` too (NaN equal to NaN, Rust's `total_cmp` style):
  rejected for now; it changes `==` against the IEEE intuition and the
  Go backend, and equality on aggregates already follows IEEE. Revisit
  with a new ADR if maps keyed by Float need it.
- `-0.0` before `0.0` in the order (IEEE totalOrder): rejected, the Go
  backend and the spec treat them as equal and the sort is stable.
