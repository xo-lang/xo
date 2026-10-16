# arm64: a MIR pass that joins blocks must not cross shrink wrapping's boundary

Status: Observed (2026-10-16, the register allocator attempt,
bench/results/arm64-regalloc-2026-10-16.md).

Symptom: programs with recursion hang or overflow the stack at run time
(the recursion golden spun in eval), while every build succeeds.

Cause: selfhost/arm64 shrink.xo plans shrink wrapping before register
allocation. Its restore blocks (fedge 2) are where emission pops the
frame before a jump out of the framed region. A pass that merged a
restore block into its predecessor removed that jump, so the function
returned through the shared `ret` block without restoring sp, x29 and
x30. g.fl (which blocks run without the frame) is filled only at
emission; before it, the plan is g.region and g.fedge.

Fix: a pass between shrink_split and emission that joins or reorders
blocks checks `g.shrink` and keeps blocks of different `g.region`, and
any block where `g.edge_block(b)` holds, apart. Do not read `g.fl`
before emission (it is empty: an index fault). Also skip latches
place_latches marked (g.dup_ok).
