# arm64: a frame home a loop's parameter takes must be written on every entry edge

Status: Observed (2026-10-18, xo-4, branch xo4-audit2 before its push);
fixed.

## Symptom

An xo built by a compiler with frame homes (compiler/arm64 byref.xo
frame_homes_of: a large value passed as a `var` argument lives in one
place in the caller's frame) crashed with a segmentation fault in `xo
publish` (cmd/xo TestPublish, TestSubdirModules): a null load at
`for m in b.modules` in modules.ModCtx.check_effects. The fixed point
and the sweep passed; only these paths reach the shape.

## Cause

byref.xo held_in lets a frame home take a loop header's block parameter
when one incoming edge (the back edge) passes it in place, on the
understanding that the other edges store their argument into the home
(isel.xo edge_moves). edge_moves did that only for a parameter kept
only in memory (mem_only). In ModCtx.load the hashes loop's body copies
`b` out of the home after each call, so `b` was not mem_only: the
header's state still said the home held `b`, but the entry edge never
stored it. When the loop ran no time (a module with no dependencies,
so no hashes), check_effects got the address of a home holding stale
stack words.

## Fix

edge_moves stores the argument into the home for any parameter the
target block's state counts as held there (byref.xo param_home), not
only mem_only ones. Golden internal/llvm/testdata/golden/byref_home_loop
(a reduced ModCtx.load, run on arm64 by internal/arm64 TestGoldenRun).

How to find such a fault: break at the callee's entry in lldb and read
the words at its pointer argument; stale return addresses and frame
pointers there mean the home was never written. Then follow the caller's
branches into the call block for a path without the store sequence.
