# A loop's variables showed <optimized out> in the debugger: loop_split's inside names had no locations

Status: Observed (2026-11-21, task #603; found by xo-4c's #597 run)

Symptom: on linux/amd64 `xo debug` at a breakpoint inside a loop showed
the loop counter as `<optimized out>` and the running sum with its
value from before the loop (cmd/xo TestDebugBatch, TestDebugStep,
TestDebugDAP). First bad commit 7a308f44a (x86-64 loop_split, task
#481), last good 3630f6b3c.

Cause: loop_split (compiler/arm64 loopsplit.xo) gives every register a
loop names a second name inside the loop, with parallel moves on the
edges in and out. The variables' location plan (dwvars.xo dv_plan,
dv_copies) describes a binding by its value's register and by the
registers that are copies of it, where a copy is a register whose one
definition is a parallel move. A split name is assigned inside the loop
as well as on the entry edge, and the outside name again on the exit
edge, so neither counted as a copy: inside the loop the variable had no
range, or only the outside name's stale one. arm64 has the same code
and splits fewer loops (11 live registers against 6 on x86-64), so no
arm64 test had a split loop under a breakpoint.

Fix: dv_copies pairs the two names of every `IF_SPLIT` edge move, both
ways, so a binding is described by each name over that name's own
range.

Avoid: a pass that renames registers after instruction selection
(shrink_split, loop_split) must say how the debugger follows the new
names; check `xo debug` locals at a breakpoint inside the changed
region on the architecture where the pass fires most.
