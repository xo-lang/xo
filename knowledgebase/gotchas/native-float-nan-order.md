# Native backends order NaN differently from the Go backend

Status: Observed (2026-10-08, found writing `internal/arm64/testdata/run/select`).
Fixed 2026-10-08 (decisions/0053).

Symptom: a program comparing Floats with `<` where some values are NaN
(`0.0 / 0.0`) printed different results under the Go backend than under
`--backend=llvm` and `--backend=arm64` (which agreed with each other).
Writing the golden test for the fix found three more differences:
`(-0.0).min(0.0)` was `-0.0` on Go and `0.0` natively; a recursive
(boxed) value holding a NaN was equal to itself natively but not on Go;
and the literal `-0.0` was positive zero on the Go backend only.

Cause:
- `spec/std.md` makes `Float` `Ord` with NaN last; the Go backend lowers
  `<` through `rt.FloatCmp`, the native backends lowered it to IEEE
  `fcmp olt` / `fcmp` + `mi`, false whenever an operand is NaN.
- Native `min`/`max` were `a < b ? a : b`, which returns the argument on
  a tie; `rt.Min` keeps the receiver.
- The native box equality helpers answered true for the same cell
  before comparing contents; a NaN inside is not equal to itself.
- The Go backend emitted `-0.0` as `(-float64(0.0))`, a Go constant
  expression, and Go constants have no negative zero.

Fix: decisions/0053 writes the rule into `spec/core.md` 2.1 and
`spec/std.md` 1 (IEEE `==`, total order for `<` and friends, receiver
on a tie, `-0.0` negative). `floatOrder` in `internal/llvm/fromssa.go`
and `internal/arm64/isel.go` lowers the ordering compares and
`min`/`max`; the box helpers compare a cell holding a Float
(`ssa.Type.HasFloat`); the Go backend negates through `rt.FNeg`.
`internal/llvm/testdata/golden/float_order` checks every pair of
NaN, +-Inf, +-0.0 and numbers, both as values and as branches, against
constants, min/max, aggregates, two sorts, and 4000 random pairs, on
all three backends.

How to avoid: after `fcmp`, only mi, ls, gt, ge (and eq) are false on
NaN; lt, le, hi, hs, ne are true on it. Pick the condition from the
four flag states (less N, equal ZC, greater C, unordered CV), not by
analogy with integer compares, and run any new Float lowering against
`float_order`.

Related (Anticipated): `internal/arm64` caches constant registers in a
map keyed by the constant's `float64` value, where `-0.0 == 0.0`. The
SSA builder never makes a `-0.0` constant today (unary minus stays an
`OpNeg`), but a constant folding pass that does would reuse the `+0.0`
register for `-0.0`; key Float constants by their bits first.
