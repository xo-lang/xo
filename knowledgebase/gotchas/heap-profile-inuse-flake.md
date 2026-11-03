# TestProfHeap's in use count is sometimes 2.5 MB in 2585 objects

Status: Observed (2026-11-03, linux/arm64 offload host 2; cause not found yet)

## Symptom

internal/llvm TestProfHeap/arm64 fails with `in use 2.50MB in 2585 objects, want about 2 MB in
2000`. Every in use byte is raw.str_repeat's, under keep: 585 of keep's temporary
`"k".repeat(1000)` strings stay counted as live at rate 1, as if their frees (or moves) never
reached the profile. The allocated object count also varies by about 30k between runs of the same
program (136k to 167k at rate 1). Tier 41 hit it at 3594a63c7.

## What is known

- It predates the reserve change it was first blamed on (e68bf2fac): its parent 152fbd965 failed
  1 time in 9, and e68bf2fac itself passed 10 of 10.
- It also failed once with XO_THREADS=1, so it is not a race between worker threads.
- It came about once in 100 runs while host 2 was busy with other jobs (tier 41), then 0 times in
  850 on demand: the program alone (whole program and separately compiled builds, every XO_THP
  mode) and the test under CPU contention or beside an uncached self build.
- In other failing runs the in use count was right (2.00 MB in 2068); which later check failed
  was not kept.

## What to do

TestProfHeap now keeps a failing run's directory (xo-prof.xoraw, the program's output) and the
program in `$TMPDIR/xo-profheap-fail-*` and logs the path. When it fails again, read the raw
profile's live slots for the extra str_repeat samples: their addresses (duplicates of a live one, or
blocks freed through a path that skips heapprof.freed: llrt/heapprof.xo forget and fold, alloc.xo
resize of a sampled block).
