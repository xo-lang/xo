# A negative literal is a negation in the SSA

Status: Observed (2026-10-12, internal/arm64 and compiler/arm64, decision 0194)

Symptom: `a / -7` (and `a / (-7)`, and a `const M: Int = -7` divisor) did
not take the arm64 backend's division by a constant lowering: it emitted
`mov x9, #7`, a checked `negs` with a branch to `xo_fault_neg`, the
divide by zero and overflow checks, and an `sdiv`.

Cause: the SSA builders (internal/ssa build_expr.go, compiler/ssa
expr.xo) lower unary minus to `OpNeg` of the constant without folding,
so `constInt` of the divisor fails, and the negation of a constant gets
the overflow check of any Int negation. LLVM folds it itself.

Fix: internal/arm64 `negConst` (compiler/arm64 `neg_const`) reads an Int
negation of a constant other than the minimum as a constant: `OpNeg`
of a constant becomes that constant (no check), and `divRem` takes it
as a constant divisor. Folding it in the SSA builders instead would
change the SSA goldens of every program with a negative literal; other
backend sites that use `constInt` (immediates of compares and adds)
still see the negation.
