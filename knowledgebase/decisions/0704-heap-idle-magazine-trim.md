# 0704: a heap record gives back the magazines of size classes it has not used for a period

Status: Accepted (owner 2026-11-17, lead decision #97; proposed by xo-lang-1, task #213). The spare rule is superseded by 0714 (every pass, with the free list).

## Context

A heap record (one per thread) keeps, for each prefixed size class (1 to 76, blocks under 16 KiB), a free
list of up to a magazine's worth of blocks (MAG_BYTES 64 KiB, at least 8 blocks) and one spare magazine; a
free that fills the list moves the old spare to the central list and the list becomes the spare. Nothing
returns a record's list or spare while the thread keeps running other classes, and the sweep (decision 0676)
gives back only 4 KiB units covered by free blocks on the sweeping record's own lists and the central lists.
So a long running server whose threads touch many classes once in a while keeps up to 2 x 64 KiB a class a
thread resident: std/sse's 30 minute stress (task #213, bisect9, 2026-11-17) kept about 47 MB on free
lists on both backends (arm64 40 -> 84 MB RSS, llvm 41 -> 85 MB), about 1 MB in each of about 40 classes
over 18 records, with 3 to 4 MB central.


## Measurements (2026-11-17)

Linux arm64 host 2, 30 minute stresses, base 2edaebbf5 against the prototype (branch p0704, ac34f9915; RSS
at the mark is the median of minutes 5 to 7, at the end the median of the last 3 minutes):

| stress | base mark -> end | prototype mark -> end |
|---|---|---|
| std/otlp (32 tasks) | 64 -> 76 MB (+20%, FAIL) | 60 -> 66 MB (+10%, pass) |
| std/sse (32 subscribers) | 24 -> 53 MB (+122%, FAIL) | 25 -> 56 MB (+122%, FAIL) |

std/bufio's growth was task #213's preemption bug (1ccfd45e1), not magazines, and is no longer a case.
Host 1 `xo bench --ab`, base against prototype: bench/perf/json wall 1.016 (0.95 to 1.064, n=3),
mapops 1.021 (0.986 to 1.039, n=5), httpkeep 1.008 (0.965 to 1.03, n=5): all within noise.

So the trim bounds otlp's growth and costs nothing measurable, but it does not explain sse's: sse's
growth is not idle classes' magazines (the prototype gives those back) and needs its own accounting (each
record's per class list and spare bytes, the central lists and the live blocks at the end of a run) before
a fix; that is the next experiment, outside this decision.

Landed with the unit test `an age pass gives an idle class's spare magazine to the central list`
(std/native/llrt/heapos_test.xo), which fails before the change (the spare stays on the record).

## Decision

At each age pass (decision 0533, pool.xo age_check, run by the record's own thread), a record gives a
prefixed class's spare magazine to the central list when the spare is the same magazine the previous pass
saw (H_IDLE, one word a class): no free filled the list and no refill took the spare for a whole period
(SCAV_EVERY). The central list then holds its blocks, the same pass's sweep counts them, and units wholly
covered by free blocks go back as before. A busy class's spare changes every magazine's worth of frees, so
the hot paths (alloc, free) are unchanged and pay nothing; a record that needs the class again refills from
central under the class lock, as any refill does.

## Consequences

- Footprint of long running servers bounded by what classes are in use in each period, not by every class
  a thread ever used.
- One extra word read and written per class per age pass per record (76 words), and a central lock taken
  only for classes that idled.
- A record's partial free list (under a magazine) still stays with the record; a later step could hand it
  over too (it needs a partial magazine kind on the central list).

## Alternatives

- Every spare to central at each pass (task #213 bisect5 experiment): moves busy classes' spares too, which
  a refill takes straight back under the lock: lock traffic for nothing.
- One magazine a class for small classes (no spare): halves the bound but makes every magazine's worth of
  frees take the central lock (the spare exists to absorb alloc/free ping-pong at the boundary).
- Per class high-water trim counting takes on the hot path (Go's mcache flush at GC): costs the alloc path
  an increment; Xo has no GC cycle to hang the flush on.
