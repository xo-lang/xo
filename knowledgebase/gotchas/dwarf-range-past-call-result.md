# A variable's DWARF range ran past its call into the result moves

Status: Observed (2026-11-05)

## Symptom

After returning from a call (lldb `finish`, `xo debug`'s finish), a local
of the caller that had been passed in x0 showed the call's result: in
`let n = ...; let f = fact(n)`, main's `n` read 6 at the return address
(fact(3)) instead of 3 or unavailable.

## Cause

Native variable ranges (compiler/arm64 dwvars.xo dv_plan) come from each
virtual register's live range and were labeled at instruction boundaries.
An argument's range ends at the call instruction, but one K_CALL emits
both the `bl` and the moves of its results, so the range's end label
came after `mov x2, x0`, and the return address (between them) was still
inside it with x0 already holding the result. Clipping the range at "the
call instruction" changed nothing: that was already its end.

## Fix

dv_plan's positions are half instructions: 2 * k before instruction k,
2 * k + 1 the return address of call k. A span in a caller saved register
(dv_caller_saved) that reaches a call ends at 2 * k + 1, and the emitter
writes that label right after the `bl` or `call` (G.dv_ret); where a call
does not write it, it falls to the next instruction (the old end). Fault
stubs' DvStub.at uses the same units. Test: internal/arm64
TestDebugVarsAfterCall (lldb finish, arm64 and LLVM).

Lesson: a debug range keyed to instruction indexes cannot end inside an
instruction; check what one IR instruction emits before clipping at it.
