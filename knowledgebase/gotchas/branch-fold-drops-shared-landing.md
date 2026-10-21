# Folding a branch can drop a landing check a live call shares

Status: Observed (2026-10-21).

Symptom: with branches decided by the operands' bounds folded in
compiler/arm64 isel (first in branch_op), fault_unwind failed to build
on arm64 and amd64 with `assembler: LpGL1jtk_f1_b95 is used but not
defined`: the exception table named a landing pad block the emitted code
no longer had. Earlier, ssa fold_branches in functions with landings
failed the inlined module's verification (windows.fix: "a value landing
on a check that was no longer one"), so it skipped every function with a
landing pad, which left the constant compares of inlined helpers in
loops (portable u64's ints.u64_rem `b > 0`).

Cause: a value's landing (Value.pad) is a check value, and the pad block
is that check's branch's first successor (state.xo pad_target).
chain_pads lets calls share one check, so a call on one side of a branch
can land on a check on the other side. Folding the branch makes the
check's block unreachable; the live call still names it, its pad block
is reached by nothing, and the MIR thread drops it (or SSA verify
rejects it). Keeping the block alive is not enough: liveness into a pad
comes from the landing jump's second target, which is gone.

Fix: fold a branch only when every value in a block still reached has
its landing check in a block still reached (ssa.pads_reached, a
reachability walk with the branch taking only the kept edge); both ssa
fold_branches and arm64 fold_decided (bounds decided branches, folded in
the SSA before lowering) use it. fault_unwind's `requires n > 0` with
n = -3 inlined is the regression test.
