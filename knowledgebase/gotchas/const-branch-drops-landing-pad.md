# A constant branch dropped the block a kept call lands on

Status: Observed (2026-10-21)

**Symptom.** arm64 build fails with `assembler: L..._fN_bM is used but not
defined`, LLVM and Go pass. Seen with an inlined `ensures` that always
fails (`buggy(n)` returning `n` with `ensures result > n`) while an owned
value (a Str, a user capability box) was live: the golden contract_ensures
broke when `os.stdio.println` began holding a Box (decision 0307); base
reproduced it with a live Str.

**Cause.** isel's branch_op lowered a branch on a constant to a jump to the
taken edge only. The calls inlined on that path carry the call's landing
check, which sits in a block past the other edge, so the block was never
selected and the unwind table named a missing label.

**Fix.** branch_op keeps the dropped edge's block as a second jump target
when `ssa.pads_reached` says a kept value lands there (as the
`rt.landing` branch does). Golden: ensures_dead_pad.
