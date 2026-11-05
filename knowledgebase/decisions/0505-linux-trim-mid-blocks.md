# 0505: mem_trim on Linux: the freed class blocks and the 1 to 2 MiB cache

Status: Accepted (owner, 2026-11-04). Proposed by Xo Lang 2 the same day
at the lead's request. Extends decision 0448's trim (debug.mem_trim).
Finding 3 (the classes' doubling) stays a separate measurement item.

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

## Addendum (2026-11-04, lead-approved): every worker trims its own record

Building part 1 showed `mem_trim` reaching none of a program's freed
class blocks: a heap record's free lists are its thread's alone, and a
program's tasks (main's too, after its first park) free onto whichever
worker ran them, while `trim_thread` walked only the calling thread's
record. Instrumented on linux/arm64 (XO_THREADS=1 as well): the probe's
64 freed 64 KiB blocks sat on the main OS thread's record while the trim
ran on a worker's, whose lists and the central lists were empty. Darwin's
trim of decision 0448 has had the same gap for programs; xo's own phase
trims run on the compiling thread, so its measurements stood. Walking
another thread's record is not safe (its owner pops and pushes without a
lock, so an madvise could zero a block just handed out).

So `mem_trim` now:

1. trims the calling thread's record (part 1 above);
2. adds one to the pool's trim epoch (pool record word P_TRIM_EPOCH_I)
   and records it as seen in its own worker record (W_TRIM_SEEN);
3. takes every idle worker off the idle list and wakes it as wakep does
   (counted spinning), and breaks the network poller's waiter (np_break);
4. waits up to 20 ms for the workers it woke to acknowledge;
5. unmaps the cached large blocks of 1 MiB and over (part 2).

Each worker checks the epoch at the top of its loop (worker_main and
root_main, before it takes work or goes idle: the point agreed with
decision 0502's change, which only adds a `continue` back to it) and,
when it moved, trims its own record (rt.heap_grow TRIM_X, an explicit
trim: the Linux advice too) and records the epoch. A worker busy in a
task trims at its next switch, which may be after `mem_trim` returns;
the wait covers only the woken idle workers, so a caller whose workers
are all busy (xo's parallel build at its phase ends) pays no wait. The
idle worker's own periodic trim (pool.xo, after FIBER_TRIM_US) stays
darwin only on Linux's side: an idle server's next burst would pay the
refaults.

A second gap of the same kind: before a program's first spawn the
allocator uses the single threaded heap record (obj.xo's heap word),
which no thread's TLS_HEAP names, so `trim_thread`'s `thread_heap()`
made a new, empty record and trimmed that (instrumented: the probe's
blocks on the record made at start, owner thread unset, and a fresh
record for the trimming root thread). `trim_thread` now takes the
allocator's record: the single threaded one while it is set, else the
thread's own, and does nothing for a thread that has none.

A third: full magazines of page blocks wait on the central lists, owned
by no thread. Linux does not scavenge the 16 KiB class's magazines as
they arrive (alloc.xo scavenge_pages), nor the larger classes' first
SCAVENGE_KEEP, so a threaded program's 16 KiB blocks stayed resident
(TestMemTrimOtherThreads: 148 MB before the trim and after). `mem_trim`
now walks each page class's central list under its lock and advises
every magazine (heapos.xo trim_central; scavenge marks each block
PADVISED once).

Test: cmd/xo TestMemTrimOtherThreads (linux): 160 MB of 64 KiB blocks
freed by eight tasks, then `mem_trim` from main, at least 100 MB and half
of the resident set given back.

## As built (2026-11-04)

The probe (160 MB freed in blocks of one size, then `mem_trim` against
Go's `debug.FreeOSMemory`; ~/xo-lead/notes/xo-lang-2/trimprobe, the
driver samples after each phase). Resident set after the trim on
linux/arm64 (offload host 2); phys_footprint on darwin/arm64 (the Mac:
MADV_FREE_REUSABLE pages leave the footprint at once and the RSS only
when memory is short, so darwin's RSS column shows no trim for Xo's
class blocks or for Go).

| block | linux Xo before | linux Xo after | linux Go | darwin Xo after | darwin Go |
|---|---|---|---|---|---|
| 16 KiB (x 10,240) | 324 MB | 4 MB | 8 MB | 3 MB | 15 MB |
| 64 KiB (x 2,560) | 332 MB | 13 MB | 6 MB | 2 MB | 12 MB |
| 1 MiB (x 160) | 17 MB | 1 MB | 7 MB | 2 MB | 12 MB |
| 2, 4, 16 MiB | 1 MB | 1 MB | 5 to 8 MB | 2 MB | 5 to 55 MB |

Xo's 16 KiB row maps 326 MB for 160 MB of blocks (a 16 KiB block plus its
header takes the 32 KiB class): finding 3, measured separately.

cmd/xo TestMemTrimOtherThreads (linux/arm64, host 2): a01773c2d (main
before this change) 148 MB resident before the trim and 148 after, FAIL;
this change 162 to 8 MB (arm64) and 154 to 10 (LLVM), PASS.

`xo bench --ab` on linux/arm64 (host 1, 10 rounds, base bef074fef against
this change at 8f710d8ec; later commits are docs only):

| run | wall B/A | cpu B/A | max RSS B/A |
|---|---|---|---|
| self (xo building compiler/xo) | 1.002 (0.992 to 1.003, within noise) | 0.998 (within noise) | 0.932 (0.925 to 0.942, better): 4,449,148 to 4,147,090 KB |
| bench/perf/alloc mixed | 0.988 (within noise) | 0.986 | 1.0 |

The self run's host was busy (median load 6.9; ab-host warns that wall and
cpu ratios are less reliable then); max RSS does not depend on load. xo's
own `debug.mem_trim` calls after its phases now advise on Linux, the max RSS
gain, with no wall cost seen in the refaults the 2026-10-31 stage1 runs
warned of (those were the automatic trims, which stay darwin only).
