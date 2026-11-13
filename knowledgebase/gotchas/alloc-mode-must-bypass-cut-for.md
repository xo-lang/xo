# An allocator mode that skips the class refill must skip heap_grow's cut_for too

Status: Observed (Xo Lang 1, 2026-11-13, decision 0627's stash probe)

## Symptom

A self build with a probe allocator hung for an hour at about 300% CPU
and 92 MB resident; every emit thread sampled in alloc, cabi.heap_grow,
alloc (`sample PID` on macOS shows it).

## Cause

alloc.xo alloc carves from the bump chunk when the class list is empty
and no magazine or central refill applies; when the chunk is used up it
calls rt.heap_grow, whose cabi.xo heap_grow first tries
heapos.cut_for (cut a larger class's central magazine into blocks of
this class) and calls alloc again. cut_for leaves the blocks where the
normal refill finds them; a mode that made alloc skip that refill (the
probe's stash) never saw them, and cut_for kept succeeding, so alloc
and heap_grow called each other forever without carving a chunk.

## Fix

Any alloc mode that bypasses the magazine or central refill must also
make heap_grow skip cut_for (and carve a new chunk), or take the refill
cut_for fills. Run a new allocator mode's first build under an alarm
(`perl -e 'alarm 1500; exec @ARGV' ...`) so a loop ends on its own.
