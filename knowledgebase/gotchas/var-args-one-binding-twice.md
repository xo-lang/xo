# One binding passed to two `var` parameters differs natively

Status: Observed (2026-10-09, list_value_semantics in TestGenGoldensNative).
Resolved 2026-10-09 by decision 0111 (`XO0503`).

Symptom: `both(al, al)` with `fn both(var a: List[Int], var b: List[Int])`
printed `[7, 2, 3, 7]` on the Go backend and `[7, 2, 3]` natively.

Cause: the Go backend passes `var` arguments by reference, so both
parameters are one value; the native backends copy each in and out, and
the last write back wins.

Fix: decision 0111 makes such a call a checker error on every backend
(`XO0503`, core 5.2): the same variable, or overlapping places (`x` and
`x.f`, `xs[i]` and `xs[j]` unless the indexes are different literals),
passed to two `var` parameters. Copy one into a temporary. The native
XO1201 for this case was removed.
