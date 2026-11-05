# 0505: mem_trim on Linux: the freed class blocks and the 1 to 2 MiB cache

Status: Proposed (Xo Lang 2, 2026-11-04, at the lead's request). Extends
decision 0448's trim (debug.mem_trim).

## Context

`debug.mem_trim()` (decision 0448) gives the system the pages of the
calling thread's free heap blocks of 16 KiB and over on darwin only, and
on every target unmaps the cached large blocks of 2 MiB and over.
heapos.xo's note on the size classes' limit says Linux needs no more:
"Linux gives the freed class blocks' pages back by itself". The
untested-features run of 2026-11-04 measured otherwise.

The probe (~/xo-lead/notes/xo-lang-2/trimprobe, run by
census/trimprobe.sh): COUNT blocks of SIZE bytes, written and held, then
freed, then `mem_trim()` twice, with the process's RSS read by a parent
at each phase; then the same blocks allocated again right after the trim,
and once more warm. The Go twin does the same with `runtime.GC()` at
"freed" and `debug.FreeOSMemory()` at "trimmed". Xo at a01773c2d; about 160 MB of payload each.

linux/arm64 (offload host 2), RSS in MB:

| blocks | Xo held | Xo freed | Xo trimmed | Go held | Go trimmed |
|---|---|---|---|---|---|
| 40 x 4 MiB | 161 | 33 | 1 | 166 | 6 |
| 80 x 2 MiB | 161 | 33 | 1 | 168 | 9 |
| 10 x 16 MiB | 161 | 33 | 1 | 165 | 5 |
| 160 x 1 MiB | 161 | 17 | 17 | 165 | 6 |
| 2560 x 64 KiB | 332 | 332 | 332 | 164 | 4 |
| 10240 x 16 KiB | 324 | 324 | 324 | 168 | 8 |

darwin/arm64 (the Mac; RSS there is confounded by memory compression of
the uniform bytes, and Go's RSS stays at 165 MB after FreeOSMemory since
darwin takes MADV_FREE pages lazily): 4, 2 and 16 MiB blocks go from 34
to 2 MB at the trim; 1 MiB blocks stay at 19 MB. Blocks of 64 KiB and
over are not size class blocks on darwin (its classes' limit is 64 KiB,
0448 addendum), and trim_thread gives back the 16 KiB to 64 KiB ones.

Three findings:

1. On Linux a freed size class block (up to 256 KiB, the classes' limit
   there) keeps its pages for the life of the process: 324 to 332 MB
   stay resident after the payload is freed and trimmed, where Go is
   back to 4 to 8 MB. Linux does not give them back by itself (the
   heapos.xo note is wrong).
2. Freed blocks of 1 to 2 MiB stay cached and resident through the trim
   on both systems (17 to 19 MB here); Go returns them.
3. Separate from the trim: a 16 KiB or 64 KiB payload maps about twice
   its size (330 and 338 MB mapped for 160 MB), on Linux where they are
   class blocks. The classes above 1 KiB are powers of two less 8 bytes
   (heapos.xo class_bytes: 16376, 32760, 65528, 131064), so a power of
   two payload plus its header takes the next class. Go's classes are
   about four per doubling, and 16 KiB fits its class exactly.

The cost of giving pages back is a page fault per page at the next use.
The probe's re-allocation time is dominated by filling the blocks
(Bytes.repeat copied once per repetition; fixed separately on 2026-11-04,
std/changes/prelude.md), so the fault cost is measured again with the
fixed repeat before this is decided: Go's numbers on Linux, after the
trim against warm, were 10.8 against 6.6 ms (4 MiB blocks), 8.0 against
6.0 (2 MiB), 10.3 against 8.6 (16 MiB), and no difference for the
smaller blocks.

## Decision (proposed)

1. On Linux, `mem_trim` runs trim_thread too: `madvise(MADV_DONTNEED)`
   on the pages of the calling thread's free class blocks of 16 KiB and
   over, as darwin's MADV_FREE path does (the page holding a block's
   free list link stays).
2. On every target, `mem_trim` also unmaps the cached large blocks of 1
   to 2 MiB (the large block cache's smallest bins).
3. heapos.xo's note is corrected to the measurement.
4. Finding 3 (the classes' doubling above 1 KiB) is not decided here: a
   follow-up measures intermediate classes (Go's spacing) on the self
   build and the std twins.

Acceptance, with the probe on linux/arm64 and darwin/arm64 against Go:
RSS after the trim within 2x of Go's for every row above (Linux), no
darwin row worse than today, and the re-allocation after a trim within
1.5x of warm, with the time a program spends without trimming unchanged
(`xo bench --ab` on the self build and bench/perf/alloc, since
trim_thread runs only when called).

## Consequences

- A long-running Linux server that calls `debug.mem_trim()` after a
  burst gets its medium blocks' memory back, as Go does on a GC cycle.
- The trim takes longer on Linux (a walk of the thread's free lists and
  one madvise per run of free pages); it is called rarely.
- No change to programs that never call it.

## Alternatives

- Give the pages back on free (no trim needed): the 0448 addendum
  measured free-time return as costly on darwin (8 KiB limit: 15% wall
  on the self build); Linux's MADV_DONTNEED is cheaper than munmap but
  still a system call per free.
- Lower the Linux classes' limit to 64 KiB, as darwin's: rejected in the
  0448 addendum for Linux (wall 1.115 worse at 16 KiB, measured as "no
  RSS change", which this probe contradicts for a freed working set).
- A background scavenger as Go's: more machinery than one call needs;
  can come later on this trim's code.
