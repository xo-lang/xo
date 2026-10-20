# The faulting frame's variables showed <unavailable>

Status: Observed (2026-10-20, Xo lang 4)

Symptom: XO_TRACEBACK=locals (decision 0268) printed every variable of
the frame that faulted as `<unavailable>` on arm64, while its callers'
variables printed.

Cause: two, in compiler/arm64 dwvars.xo. A traceback's innermost frame
returns into the out of line fault stub (after the body), and the
variables' location ranges, made from register live ranges, ended at
the faulting instruction. And a binding record names the register of
its SSA value; a parameter copied into a callee saved register for a
loop with calls (`mov x22, x1`) was described only up to that copy.

Fix: each fault stub carries the places the faulting frame keeps from
its faulting instruction to its call's return (DvStub, dv_stub_expr):
callee saved registers and sp slots (sp moved 16 in a leaf's stub, and
slots are left out where the stub's prologue moved it). A register
whose one definition is a parallel move from a variable's register is a
place of that variable too. Still unavailable: a value in a caller
saved register at the fault (the stub's moves and the fault function
overwrite them; decision 0268's stage 2). Inlined functions' variables
(dwvars.xo skipped them) are described under their
DW_TAG_inlined_subroutine since stage 3, and the runtime's reader shows
an inlined copy outside its call's range where its list places it (the
stub, past the body). internal/llvm TestFaultLocalsFaultingFrame,
TestFaultLocalsInlined.
