# Word copies within the frame stayed word by word when the ranges overlapped

Status: Observed 2026-11-10 (decision 0564 step 3, Xo Lang 1)

## Symptom

A wide value moved within the frame (an sret call's result into a call's
outgoing arguments) cost one ldr/str pair per word: with 0564 step 3,
core.run_doc had six runs of 611 pairs (`ldr x17, [sp, #232+8k]` then
`str x17, [sp, #8k]`), and the self build was 14% slower.

## Cause

copy_runs_l (compiler/arm64 lsr.xo) turns a run of x16/x17 word copies
into q-register block copies, but copy_block took two sp ranges only when
they were apart. A move down the frame by less than the value's size
overlaps.

## Fix

copy_block also takes overlapping sp ranges with the stores below the
loads (a forward move: each 32 bytes stored end at or below the next words
loaded). A move up the frame with overlap still goes word by word; it
would need a descending block copy. To find such runs, scan `xo dev dump
asm` output for long `ldr x16|x17, [sp, #a+8k]` / `str x16|x17, [sp,
#b+8k]` sequences (`--fn` there names build roots, so the dump holds every
function reached: filter by the function's label).
