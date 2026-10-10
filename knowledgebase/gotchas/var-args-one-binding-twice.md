# One binding passed to two `var` parameters differs natively

Status: Observed (2026-10-09, list_value_semantics in TestGenGoldensNative)

Symptom: `both(al, al)` with `fn both(var a: List[Int], var b: List[Int])`
printed `[7, 2, 3, 7]` on the Go backend and `[7, 2, 3]` natively.

Cause: the Go backend passes `var` arguments by reference, so both
parameters are one value; the native backends copy each in and out, and
the last write back wins. The spec does not say which is meant.

Fix: the SSA builder reports XO1201 ("one binding passed to two `var`
parameters", planned) when two `var` arguments of a call (or a `var`
receiver and an argument) share a binding and one path of fields and
indexes is a prefix of the other. Hybrid builds keep such a function in
Go. Deciding the semantics (reject it in the checker, or define copy in
and out) needs an ADR.
