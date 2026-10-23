# A call that does not return left its landing pad unreached

Status: Observed (2026-10-23)

**Symptom.** arm64 and amd64 native builds fail with `xo: assembler:
LpGL1jtk_f1_b74 is used but not defined`; xo check is clean and LLVM
builds and faults as expected. Found by xo-deepseek-2:
`csv.parse("a,b\n", sep: '"')` (a constant invalid separator) in a test
or inside `task.isolate`; sep ';' and csv.format with sep '"' built fine.

**Cause.** The inlined check_delims proves the fault, so the block that
calls `rt.fault_at` (no return, then a trap) is reached and the block
holding the `rt.landing` branch to the pad is not. The pad was a successor
only of that landing branch: MIR successors (mir.xo msuccs) came from a
block's jump or branch terminator alone, so the emitter's reachability
walk skipped the pad while the call site's unwind entry still named its
label. branch_op's `ssa.pads_reached` keeping (const-branch-drops-landing-pad.md)
does not cover it: the branch was decided through a block parameter, not
a constant operand of the branch.

**Fix.** msuccs makes the landing pads of a block's calls its successors
when the block ends in neither a jump nor a branch, so liveness, register
allocation and the emitter all see the pad. Golden: fault_pad_proved
(arm64, LLVM, Go parity, Docker amd64).
