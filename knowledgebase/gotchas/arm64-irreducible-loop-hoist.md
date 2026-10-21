# arm64: a constant hoisted out of an irreducible region is read before it is made

Status: Observed (2026-10-21, Lib 1's flate.InStream.push; fixed the
same day)

## Symptom

`xo: arm64 backend: internal: flate.InStream.push reads virtual register
307 before it is written`, while LLVM and Go compile the function. One
long method: a `while` with a state machine, match arms that `break` or
`continue`, a tuple `let`, `?`. Removing any one region of the function
made it compile, so it looked like a size threshold.

## Cause

compiler/arm64 opt.xo dfs_loops took every retreating edge of its depth
first search for a back edge. In an irreducible region (a cycle with
two entries, which the lowered state machine had) the edge's target
does not dominate its source, so the "loop body" (the blocks reaching
the latch without passing the target) included blocks some paths reach
without the "header". hoist_consts put the `% 31` magic number in that
header's single outside predecessor, which those paths never pass.
Not dependent on decision 0300 (an xo from before it fails the same).

## Fix

dfs_loops drops a loop whose backward walk reaches the entry block (the
exact test of the target dominating the latch), so hoist_consts and
the other users of it see natural loops only. Unit tests:
compiler/arm64/loops_test.xo. To find such a case: `XO_PASSES=-NAME` per
pass (`hoist_consts` off made it build), then the block of the
register's definition against the block of its read.
