# A never-taken call in a hot loop slows the loop on arm64

Status: Observed (2026-10-21, Xo Lang 3, decision 0291's hybrid Map)

Symptom: a Map get loop with one more branch, to a call that never runs
(the hybrid Map's out-of-line log lookup), took about 1.5x the time of
the lists' loop although it retired only 1.19x the instructions.

Cause: the call, even in a cold block, clobbers the caller-saved
registers, so arm64's register allocator keeps the loop's live values
in callee-saved registers (more saved in the prologue) and stops
hoisting loop invariants: the loop rebuilt a constant (movz/movk), the
static base (adrp/add), and the lists' element bases each iteration,
which the call-free lists loop computed once.

Fix (open, codegen; Xo lang 4): allocate around cold calls (spill and
reload at the cold call, not across the loop). Until then, keep hot
loops free of calls: inline what is small, and put a rare path behind a
branch to code outside the loop body when possible.
