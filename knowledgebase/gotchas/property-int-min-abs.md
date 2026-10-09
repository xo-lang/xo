# Property tests generate `Int.MIN`, and `n.abs()` faults on it

Status: Observed (2026-10-08, `selfhost/lexer/lexer_test.xo`)

Symptom: `test "..." for n: Int { let v = n.abs() % 1000 ... }` fails on
case 49 with `n = -9223372036854775808` and `fault: integer overflow:
abs(-9223372036854775808) does not fit Int`.

Cause: the Int generator includes the edge values, and `Int.MIN.abs()` does
not fit in an Int; Xo faults on overflow instead of wrapping like Go.

Fix: reduce before taking the magnitude: `(n % 1000).abs()`, or
`(n / 2).abs()` for a non negative value over the full range.
