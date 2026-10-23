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

**First fix crashed amd64 (2026-10-23).** c0d328e3e made `inst.pad`
itself the successor. A call's pad can be a block that only jumps,
which the layout drops (the emitter resolves pads through `pad_of`).
shrink's liveness indexed that block outside the order, and amd64
builds of ordinary programs crashed ("index -1 out of range" at
arm64/regalloc.xo): bench/perf/portable, sql_conv, sql_wire_names and
xml_basic, for linux, windows and darwin. It was reverted (751f0fe7b).
The re-land uses `pad_of(inst.pad)`, shrink skips blocks outside the
order as the other liveness passes do, TestAmd64CrossBuilds builds
those four programs for the three amd64 targets, and the Docker sample
runs the three goldens. A MIR or regalloc change gates on that test.
