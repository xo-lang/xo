# An index that is an upper case constant parses as type arguments

Status: Observed (2026-11-19, task #410)

Symptom: `COEF[LAST]`, where LAST is an Int constant, fails to build with
XO0407 "type arguments are only allowed on generic functions and types"
and "`LAST` is not a type".

Cause: the parser reads `NAME[Upper]` as an instantiation with type
arguments when the bracketed operand is a single upper case identifier,
before the checker knows that LAST is a constant.

Fix: write the index so it is not a lone upper case name: a literal
(`COEF[3]`) or an expression (`COEF[LAST + 0]`; parentheses, `COEF[(LAST)]`, do not help). The
constant index read of a list constant is folded either way (task #410).
