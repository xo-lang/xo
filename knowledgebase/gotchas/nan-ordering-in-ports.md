# Code ported from Go compares NaN differently

Status: Observed (2026-10-20, decision 0287)

Symptom: std's port of Go's math matched Go on 21,465 cases except
nine, all with a NaN argument: `tanh(NaN)` returned 1 and `hypot(NaN,
0.7)` returned +Inf, where Go returns NaN.

Cause: Xo's `<`, `<=`, `>`, `>=` on Float are a total order with NaN
after +Inf (core 2.1: `NaN > x` is true for every non NaN `x`), so
sorting and `Ord` work; Go's are IEEE 754, false whenever an operand is
NaN. A line-by-line port of `if f > MaxFloat64` (Go's IsInf) or `if z >
0.5*MAXLOG` (tanh) takes the branch for NaN in Xo and not in Go. `==`
and `!=` are IEEE in both, so equality tests port unchanged.

Fix: in ported numeric code, check `is_nan` before an ordering
comparison a NaN can reach (gomath.xo `is_inf` and `tanh`), and keep a
differential test whose inputs include NaN in every argument position.
