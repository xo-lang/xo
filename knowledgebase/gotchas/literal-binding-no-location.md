# A literal's binding had no DWARF location

Status: Observed (2026-10-22, Xo Publish).

Symptom: `XO_TRACEBACK=locals` printed `label = <unavailable>` for
`let label = "order-7"` in main, though label is used after the faulting
call. Its location list was empty.

Cause: compiler/arm64 isel.xo words makes a constant's registers again at
each use (adrp/add and mov after the call), so the binding's value has no
register live across the call. dwvars.xo's constant words (DW_OP_constu,
DW_OP_addrx) only filled a binding that also had a register word to take
its range from; an all-constant binding had none. Marking all-constant
words did not help either: the binding's registers come from a later
use's copies, which are not constant registers (kints, ksyms).

Fix: dwvars.xo dv_plan describes a binding whose SSA value is an
ssa.Op.Const (Int, Bool, string) as the constant over the whole function
(dv_const), the string's object only when the function already names it.
LLVM leaves a string literal's object out (llvm-aggregate-dbg-value.md),
so there it stays unavailable.
