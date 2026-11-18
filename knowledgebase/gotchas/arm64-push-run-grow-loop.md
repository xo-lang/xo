# A push run's growth loop made every list literal a loop

Status: Observed (2026-11-18, task #329)

## Symptom

`xo test` of std/backoff's generated fuzz corpus (seed 19, 5,000 cases
in one `const` of tuples holding `List[Int]` literals) peaked at 13.1 GB
on the Mac and hit host 2's 18 GB `XO_MAX_MEM` during `emit`. Build time
and memory grew faster than the case count: 1,000 cases 8 s, 2,000
cases 27 s and 3.1 GB.

## Cause

compiler/arm64 pushrun.xo `push_n` (a run of up to `PUSH_RUN_MAX`
pushes onto one list) checked the capacity, and its growth path jumped
back to the check, since one growth may not fit the run for a list made
with a small capacity. Every list literal was therefore a loop, and the
const initializer of 220,000 pushes had tens of thousands of loops in
one function. The loop passes (`dfs_loops`, `hoist_invariants`,
`hoist_consts`) keep a block set per loop, O(loops x blocks) in time
and memory.

## Fix

The growth path is one call, `xo_x_obj_rl_grow_to(l, n, esize)`
(std/native/llrt obj.xo `rl_grow_to`, which grows until the list holds
n), then the stores: no back edge. `internal/arm64`
TestPushRunGrowNoLoop checks the assembly: no growth block goes to a
block that branches back to it. The 5,000 case file: 3.4 GB and 22 s;
regalloc's `insert_ranges` is most of what remains.

A first fix (a second check and growth in line) kept no loop but
doubled each run's cold code; both versions let the allocator keep more
lists in registers, which left test/codegen/ladders' 20 lists with 15
spilled slots, under `LADDER_MIN` (16): that test now has 26.

## Avoiding it

A lowering that emits a loop for each source operation multiplies the
loop count by the program size: per loop passes then scale with the
square of a generated file. Prefer a bounded straight line sequence
when the trip count has a small known bound.
