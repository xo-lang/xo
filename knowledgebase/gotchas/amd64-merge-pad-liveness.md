# amd64 block merging: landing pad edges lengthen lives

Status: Observed (2026-10-17)

## Symptom

Merging fall-through blocks before the amd64 retarget (x86_merge_blocks,
so a load and its use meet for x86_fuse_load) first changed nothing in
nbody's hot loop, and then, once it did merge there, made interp 10%
slower (1.842 G to 2.034 G instructions) and sort 2.6% slower: spill
references in interp's `run` went from 87 to 159.

## Cause

In a function with a landing pad (decision 0085) each block ends in a
jump `[next, pad edge]`, the second target only for liveness, and each
pad edge is its own block with its own moves. The first version merged
only jumps with one target, so it skipped every block of such
functions. The second gave the merged jump both blocks' pad edges, which
is correct but keeps every value the first block's pad edge needs live
through the second block: longer lives, more spills.

## Fix

Merge a block with pad edges only when every value live into those pad
edges is already live out of the block it absorbs (x86_live), so no
value lives longer. nbody keeps its fusions (5.170 G to 5.020 G) and
interp merges with its old spills.
