# 0383: The heap hands back none of the memory the build frees, so xo building itself peaks at 10.4 GB with 0.2 GB live

Status: (c), the scavenge of big classes' central magazines with a threshold of 4, Accepted (owner, 2026-10-26)
and pushed as 811c4fb8d; (a), cross class reuse by cutting, approved as a branch and landed as 85d08254a
(Xo Lang 3, 2026-10-26); (b) Proposed, not built (it recovers nothing alone). Follows the
cold peak study the lead asked for after tokens as offsets proved not to be the peak lever.

## Context

Measured 2026-10-26 on the current main xo (/tmp/ss/xosa/bin/xo, a native build), cold builds, one
profiled run each, a private XO_BACKEND_CACHE_DIR.

Footprint against live memory (`--phase-times` heap and peak, and XO_PROF=heap with
XO_PROF_HEAP_AT=PHASE, which is what is alive after the phase):

| program | phase | footprint (heap / peak) | live |
|---|---|---|---|
| llvm-fib | past fe.build | 392 / 393 MB | 262 MB |
| llvm-fib | past emit | 428 / 440 MB | 66 MB |
| llvm-fib | past assemble | 414 / 461 MB (RSS 488 MB) | 55 MB (past undef) |
| xo itself | past fe.build | 3778 / 3779 MB | 2.71 GB |
| xo itself | past emit | 10385 / 11079 MB | 209 MB |
| xo itself | past hollow | 11006 / 11707 MB | n/a |
| xo itself | at exit | RSS 8.6 GB, peak footprint 10.4 GB | |

(xo itself, past emit: the heap profile lost 869 K samples, so its split by function is approximate;
the total is not.) The peak of a cold build of xo is 50 times its live set. This is the memory the
reaper stopped jobs for and the 37 GB incident of 2026-10-22.

A scratch hook (in a scratch worktree, never pushed: heapos.xo walked every heap record's free
lists, the spare magazines and the central magazines, and read each carved block's class from its
8 byte prefix) reports who holds the free memory after a phase:

- Whose lists: past emit on xo itself 7,320 MiB in 1,830 chunks, free: per-thread lists 31 MiB,
  spare magazines 61 MiB, the central lists 5,330 MiB (98%). llvm-fib past emit: 100 chunks,
  per-thread 21 MiB, spare 23 MiB, central 286 MiB (87% of 330). Past fe.build the central lists
  hold only 18 to 20 MiB, so the memory is not the front end freed at the strip: it is the
  emit phase's own churn (xo allocates 33 GB over its emit phase, through 9 heap records).
- Which classes (xo itself past emit, MiB carved, free, central):

  | classes | carved | free | central |
  |---|---|---|---|
  | 2 to 50 (32 to 800 B) | 1,405 | 823 | about 790 |
  | 65 (2 KiB) | 692 | 672 | 671 |
  | 66 (4 KiB) | 223 | 194 | 193 |
  | 67 (8 KiB) | 496 | 454 | 454 |
  | 68 (16 KiB) | 471 | 391 | 389 |
  | 69 (32 KiB) | 849 | 752 | 749 |
  | 70 (64 KiB) | 1,119 | 980 | 973 |
  | 71 (128 KiB) | 727 | 561 | 550 |
  | 72 (256 KiB) | 903 | 631 | 608 |

  Of 6,885 MiB carved, 5,458 MiB are free; the classes of 2 KiB and over hold 4,635 MiB of it
  (85%). llvm-fib past emit shows the same shape in 360 MiB: classes 65 to 67 (2 to 8 KiB) carved
  189 MiB and free 184 MiB, classes 68 to 72 carved 79 MiB and free 46 MiB.
- Which chunks: the memory is not in chunks that could be given back. Past emit, xo itself has 952
  chunks of 1,830 at least 90% free (3,781 MiB of the free memory in them) but 0 or 1 chunks wholly
  free, because a few live blocks (and the unused tail a replaced chunk leaves) pin each chunk; llvm-fib
  has 32 of 90 chunks at least 90% free (124 MiB) and 0 fully free.

The allocator (std/native/llrt/alloc.xo, heapos.xo): 72 size classes (16 B steps to 1 KiB, powers of
two 2 KiB to 256 KiB), a free list per class per thread's heap record, magazines of
MAG_BYTES = 64 KiB (at least 8 blocks) moved to central lists when a list passes its cap, blocks
carved from 4 MiB chunks, chunks never unmapped, a free block never coalesced and never moved to
another class. A thread whose class is empty takes a magazine of the same class from the central
list, else carves fresh from its chunk.

Go's allocator, for comparison (runtime/mheap.go, mgcscavenge.go): objects live in spans (runs of
8 KiB pages) of one size class; when the sweep finds every object of a span free, the span goes back
to the page heap, where adjacent free pages coalesce, and any class can take them; a background
scavenger returns free pages to the OS (madvise MADV_DONTNEED on Linux, MADV_FREE on macOS, paced
to hold RSS near the heap goal plus 10%, and GOMEMLIMIT's limit). Xo has neither of its two pieces:
no span is ever wholly returned (a class's blocks of all ages share a chunk, with no per chunk or
per span live count), and nothing is returned to the OS.

## Decision (proposed, in the order to build)

0. First a timeline, not a build: sample the per class carve and free bytes (the scratch hook) every
   100 ms through xo's own cold build, to learn whether the classes emit carves late are the ones it
   freed early. This decides how much of (a) below is reachable (it is the difference between the
   sum of the classes' peaks, what the heap holds today, and the peak of their sum, the most (a) can
   bring it to). It needs no allocator change.
1. (c) Scavenge: when a magazine of a class of 32 KiB or over is pushed to the central lists (cold
   memory: no thread's list holds it), return the pages inside each of its blocks that the block
   fully covers, except the page holding the block's link word (madvise MADV_FREE, or
   MADV_FREE_REUSABLE on Darwin; the kernel gives back zero pages that refault on reuse, which the
   allocator allows because a block's contents are undefined). Expected on xo itself past emit:
   the free blocks of classes 69 to 72 total 2,924 MiB; the page granule is 16 KiB, so per block
   a 32 KiB block gives 0 to 50%, 64 KiB 50 to 75%, 128 KiB 75 to 88% and 256 KiB 88 to 94%:
   1.5 to 2.2 GB of the 8.6 GB RSS (17 to 26%) with no change to the lists, the classes or any
   address. The cost is a madvise call per 32 KiB or larger block pushed, and a page fault per
   reused page (16 KiB), against the allocator's cost of 33 GB allocated in 17 s today.
2. (a) Cross class reuse from the central lists: a class whose own central list and spare are
   empty takes a magazine of a larger class (the smallest class with one) and cuts each block into
   blocks of its own size (each cut block needs its 8 byte prefix, written into the old block's
   payload; the leftover tail is cut into the smaller classes, or dropped for sizes under 16 B),
   before it carves. Blocks only get smaller, never merge back, which keeps the allocator's
   no coalescing rule. Upper bound on xo itself: the 5.3 GB on the central lists would be
   reachable by any class, so the heap could shrink toward the peak of the classes' sum; the
   timeline of step 0 gives the real bound. A risk: a split block can never serve a big class again
   (a long build ends with every class small), so (a) is kept to the classes of 2 KiB and over
   at first, where one 256 KiB block cuts into 128 blocks of 2 KiB.
3. (b) Whole chunk release alone recovers almost nothing, not the 2.3 to 3.0 GB the 90% figure
   suggests: 0 or 1 chunks of 1,830 are wholly free past emit and past assemble. What it takes:
   a per chunk live count (an increment on every alloc and decrement on every free, on the hot
   path; or a sweep that sums the free bytes per chunk from the central lists as the scan did,
   about 10 s on xo itself with a linear lookup and about 100 ms with 4 MiB aligned chunks), plus
   removing a released chunk's blocks from the lists. Even then the 90% chunks are held by their live
   blocks. It becomes useful only with (a) (cutting the blocks out of a nearly free chunk first,
   so the chunk drains) and is not worth building alone.

Rank by expected peak saving on xo itself: (a) first in potential (up to the 5.3 GB on the central
lists, unknown until step 0) but the least certain and the largest change; (c) first to build
(1.5 to 2.2 GB, certain, a few dozen lines, nothing else moves); (b) last (0 alone).

## Measured since (2026-10-26)

Step 0, the timeline (a scratch hook at every new chunk after the 700th, on xo itself, cold): 805 of
the 1,050 new chunks (77%, about 3.2 GB of the 4.2 GB carved after chunk 700) were mapped while the
central lists held free magazines of larger classes; 245 (23%, 1.0 GB) had only smaller ones. The class
asked was never the class that held the memory (a magazine of the same class would have been taken).
By class asked: 64 B class 157 of 157 events had larger free memory (about 2.0 GB on average), 16 KiB
69 of 71, 32 KiB 175 of 177, 64 KiB 237 of 246, 128 KiB 86 of 129, 256 KiB 0 of 177 (nothing is larger:
708 MiB of chunks only coalescing could avoid). So (a) reaches up to 3.2 GB of the 7.3 GB of chunks,
with a size floor (the late demand is the large classes: 177 of the last 245 events), and the 256 KiB
demand only by merging.

(c), built on the branch scavenge-central: self build of compiler/xo, cold, every cache off, two runs
each, the branch's xo against its parent's: peak memory footprint 8.60 GB to 6.84 to 7.02 GB (-1.6 to
-1.76 GB, -18 to -20%; 0383 predicted 1.5 to 2.2 GB), wall time equal (33.8 and 34.5 s against 34.8 and
34.6 s), system time 3.4 s to 4.7 s (the madvise calls), max RSS unchanged (macOS keeps MADV_FREE_REUSABLE
pages in RSS until the kernel reclaims them; heavy.sh's watch reads top's MEM, which is the footprint).
A MAP_FIXED remap in place of the madvise was tried: footprint 7.2 GB and RSS 7.3 GB (-1.35 GB each) but
5.7 s of system time, so MADV_FREE_REUSABLE is kept.

The first --ab of (c) (quiet, 15 rounds, A the parent): wall within noise everywhere (ratios 0.99 to 1.017)
except the alloc bench's `tasks` (20 ms): 15.8 to 24.7 ms (1.21 to 1.56, 30 rounds 1.089 to 1.625 interval),
RSS 0.61 to 0.66; the madvise call per freed 32 KiB and larger block cost more than a short program's whole
run. So (c) keeps a class's first 4 central magazines as they are and gives pages back only past that
(SCAVENGE_KEEP in alloc.xo): free_mt counts the class's central links, under the heap lock, before it
scavenges the magazine it is about to put there, which only this thread holds at that point.

A first version of the threshold read the links without the lock and was wrong: another thread can take a
magazine and reuse its first block, so the second word holds user data and the walk followed it (the s1 xo
built from the seed faulted building s2). Its numbers (an earlier version of this section) are withdrawn.
Nothing random reproduces it: three stress programs (big Int lists, big Strs of 'x', 16 workers, up to 150
runs each) passed on the racy build. The regression check is the s1-builds-s2 run of scripts/bootstrap-xo.sh
from the seed (failed on the racy build, passes on the locked one: s2 and s3 identical), and a debug hook
that widens the window would need a new rt name, so none was added.

Results with the locked threshold, same measurements: self build of compiler/xo (one run each, parent
then threshold) peak footprint 8.56 GB to 6.94 GB (-1.62 GB), system time 4.90 s to 5.04 s (+0.14 s), max
RSS 8.34 to 8.66 GB (unchanged by design). `xo bench --ab`, quiet (load 2.8 to 3.9), driver with cycles and
IPC: `tasks` made ten times longer (500 rounds, about 200 ms, same allocation shape), 30 rounds: wall 207.5
to 202.2 ms (0.974, 0.917 to 1.004, noise), cycles 0.928 (0.891 to 0.961, better), IPC 5.77 to 6.24,
instructions 0.999. The other nine programs (alloc small, mixed, grow, strings, maps, bigtasks, conc fanout,
pmap, spawn), 15 rounds: wall 0.963 to 1.028, all within noise; instructions equal or lower. The 20 ms
`tasks` run could not decide (the parent's own spread was 42 to 55%): a program that short is startup and
first-touch faults. Linux arm64 in Docker: every program's output equals the parent's, and 20 runs of the
Str stress and 10 of the list stress pass with MADV_DONTNEED.

Rc allocation churn is not the peak lever (2026-10-26). The Rc pass allocated 14 GB of the cold self build
(43%), 33.7 GB in all with the memo of rc.xo leaves/leaf_uses (6b08ea06b: insert_rc 6.75 to 6.00 s, user CPU
-3.4%, peak footprint unchanged). Its biggest line, the `Use{l, m}` objects of leaf_uses_fresh (5.9 GB), was
replaced by two parallel lists (branch rc-uses-lists, c9f1430a6, not pushed): peak memory footprint 8.80 and
8.81 GB (parent) against 8.76 and 8.86 GB (branch), heap at insert_rc 4869 to 4845 MB, insert_rc 6.15 and
6.33 s against 6.88 and 6.46 s (no faster), outputs byte equal. Because the allocator keeps what it frees
(this decision's cause), the churn only decides how fast the heap fills, not what stays live at the peak: the
peak follows the live set and the retained free memory, which (c) and (a) address. The derived path Strs
(4.06 GB) and the remaining Rc allocation sites are left alone; the 14 GB bound in the design notes does not
turn into footprint.

What (c) leaves, and the bound on (c2) (2026-10-26, main with (c), cold self build). Peak footprint 7.47 and
7.40 GB (8.8 GB before (c)). At the end of the build 7,044 MB of 7,148 MB of chunks are on free lists: 3,501 MB
in classes below 32 KiB (64 B 813 MB, 2 KiB 726, 16 KiB 552, 8 KiB 523, 4 KiB 232, the rest about 650), 3,544
MB in the classes (c) gives back. (c2), page-level scavenging of the small classes at the central list (a
per-16 KiB-page count of the free blocks of magazines on the central list, a page whose count reaches its size
madvised, central take subtracting; nothing counted on the thread-local lists): a scratch scan at the emit
phase (the heap peak) found 2,541 MiB of the small classes on central magazines (the other ~1 GB on thread
lists) and 1,397 MiB of 16 KiB pages fully covered by them, the same at assemble. So (c2) bounds about 1.4 GB
more (7.4 to about 6.0 GB) on Darwin's 16 KiB pages, more on Linux's 4 KiB; (a) bounds up to 3.2 GB with a
size floor. Scratch code: not pushed (a chunk registry and scan in heapos.xo of a scratch worktree).

(c2) built and measured (2026-10-26, branch c2-pages, not pushed). A central magazine's blocks link through
a word in each block, so madvising a page of small blocks would zero the links: a magazine of the 2 KiB to
16 KiB classes (the classes carrying 1,083 of the 1,404 MiB of fully free pages at emit: 2 KiB 311, 4 KiB
100, 8 KiB 347, 16 KiB 325 MiB) lists its blocks' addresses in its first block when it goes on the central
list, the blocks are counted into 16 KiB page counters (a two level table under the central lock), a page
that reaches 16 KiB of central free bytes is madvised, and take undoes the counts and links the blocks
again; a class counts only once its central list holds N magazines (pages given back and taken again at once
cost more than they free). Correct (a stress of 16 workers over Strs of 1.5 to 16 KB with cross thread
frees, every kept Str checked against its pattern: 64,000 of 64,000; the build from the seed). Cold self
build, two runs each, main with (c) against (c2): peak footprint 7.09 and 7.03 GB to 6.66 and 6.69 GB with N
of 2 (-0.34 to -0.40 GB, about 5%; N of 8: -0.36 to -0.65 GB), system time 5.7 and 5.4 s to 7.2 and 7.1 s
(+1.5 s, about 25,000 madvise calls), wall +3 to 4%. On the churn stress alone the footprint falls from
16.5 to 12.5 MB while system time goes from 0.09 to 4.8 s. A third of the 1.08 GB bound for the syscalls:
(c2) is not pushed; (a), reuse across classes without a syscall, is the next design.

(c2) with the system calls outside the heap lock (bounded experiment, same day): a page's counter carries a
release bit from the moment the central lists fill it until its madvise returns, and c2_take waits for the
bit before touching the page's blocks, so no thread is blocked on another's system call. Counting and
relinking without any madvise cost nothing (self build wall 34.26 s against 34.65 s, system time 5.62 s
against 5.56 s), so the cost is the calls. With the calls outside the lock: stress 64,000 of 64,000 intact in
30 runs, but the churn stress still took 2.6 s against 1.5 s (system time 4.8 s against 0.09 s, footprint 16.5
to 10.7 MB); the self build's peak footprint fell by 0.55 GB in one run and by 0.02 GB in the other (system
time 7.1 and 8.2 s against 8.2 and 5.5 s), so the saving is not reproducible above the run to run spread
(about 0.3 GB) and wall time is not within noise. Not pushed; branch c2-pages (commits 9ba212844, be4c6c11d
and the last) keeps the code.

(a) what the prefix does to splitting (found while writing the design for the owner). A block of class c is
size + 8 bytes: the class word before the payload. 2^k + 8 does not tile 2^(k+1) + 8: a 64 KiB class block is
65,544 bytes, two of them 131,088, eight bytes more than a 128 KiB block's 131,080, so cutting 128 KiB blocks
into 64 KiB ones gives one block and drops the rest; a 256 KiB block gives three of 64 KiB (a quarter
lost). The same eight bytes leave 262 KiB of every 4 MiB chunk uncarved at the 256 KiB class (15 blocks). So
(a) needs the big classes' blocks to tile exactly: payload 2^k - 8 for the classes of 2 KiB and over (a
request of 2,041 to 2,048 bytes moves up one class), after which a magazine of class c' cuts exactly into
2^(c' - c) blocks of class c. What (a) then reaches: step 0's 805 of 1,050 late chunks were mapped while a
larger class had free magazines (3.2 GB), by class asked: 64 B 157 of 157 events, 16 KiB 69 of 71, 32 KiB 175
of 177, 64 KiB 237 of 246, 128 KiB 86 of 129; the 256 KiB class (177 events, 708 MiB) only by merging
blocks, which this allocator never does (it would need an address ordered chunk map). The size floor: a
class takes from a larger one only when the larger magazine is at most 64 times its size, so a 64 B request
does not cut a 256 KiB block into 4,096 pieces that no later large request can use. The risk that remains is
the one 0383 names: a block once cut never grows back, so a run whose late demand is large (the last 177 of
245 growth events) can be starved of large blocks by an early cut; the measurement that decides is the same
self build with a counter of the cuts and a count of the large requests that find no block.

(a), the ledger measurement (2026-10-26, a scratch build of main with (c), counting only, cold self build): the
central bytes of each class from the puts and takes, and at every chunk growth event a virtual cut of whole
central magazines of larger classes, enough for 4 MiB, smallest donor first; a later real take of a class
whose stock the cuts had taken away counts as starved (the virtual run would carve instead; no recut is
modelled). With every larger class as a donor: 2,409 MiB of growth avoided and 1,619 MiB starved (5,717
takes), net 790 MiB; the same with donors limited to 64 times the size asked. But a donor of 32 KiB or over
is already given back by (c), so cutting it into small blocks makes its pages resident again and saves no
footprint, while its starvation still costs. With only the resident donors (classes 2 KiB to 16 KiB and
below, so (a) is a cut of the small classes' central magazines): 1,029 MiB avoided, 227 MiB starved (3,454
takes), net about 800 MiB at the build's peak chunk count (1,751 chunks; the same figure at 1,350
chunks), 11% of the 7.4 GB footprint. The growth is avoided only if a cut block's class gets the demand
the cut anticipated, and the exact tiling of the layout change below is not modelled (requests of 2,041 to
2,048 bytes move up a class). So (a) after (c) bounds 0.8 GB, not 3.2 GB: the 3.2 GB counted donors (c)
already empties.

(a) built and landed (2026-10-26, 85d08254a; the owner approved it as a branch that lands if the --ab is
within noise and the footprint drops). Two changes. The big classes (class 65, 2 KiB, up) are now exact
powers of two with the class word: payload 2^k - 8 (2040, 4088, ...; the largest request a class serves is
262,136 bytes, so a 256 KiB request is a big block), so a magazine of a larger class cuts into an exact
number of a smaller class's blocks. And a thread whose chunk is used up (cabi.xo heap_grow) first cuts the
central magazines of the smallest larger class below 32 KiB into blocks of the requested class, whole
magazines to the central list and the rest its own list, up to 1 MiB, before it maps a chunk (heapos.xo
cut_for; the donor classes above 16 KiB are already given back by (c), so cutting them would save nothing);
XO_CUT=0 turns it off in the same binary. The layout alone changed nothing (peak footprint 7.07 and 7.09 GB
against 7.03 and 7.12). Cold self build of compiler/xo, two runs each: peak memory footprint 7.03 and 7.03
GB (main) to 6.22 and 6.31 GB (-0.75 GB, -11%), the same binary with XO_CUT=0 7.09 and 7.00, max RSS 8.8 to
8.0 GB. Quiet `xo bench --ab`, 15 rounds, against main: alloc small 1.02, mixed 0.963, grow 0.977, strings
1.022, maps 0.986, tasks 0.78 (0.64 to 1.10), conc fanout 0.958, pmap 0.987, spawn 0.982 (wall; instructions
1.000 to 1.006, RSS equal): all within noise or better. Cold self build wall, 5 rounds, quiet-ish: 34.0 to
34.9 s (1.025, 0.953 to 1.089), instructions 0.991, cycles 0.974. Gates: the fixed point from the seed (s2 =
s3), TestRuntime*, cmd/xo's heap stats test, a size-phase allocation stress (pattern checks, cross thread
frees) with the cut on and off. The ledger predicted 0.8 GB; the build gives 0.75 GB. What it cannot reach: the
256 KiB class and the classes of 32 KiB and up (given back by (c) but never cut), and anything only coalescing
could free. With (c) and (a) the self build's peak footprint is 8.8 GB to about 6.2 GB.

A seed workaround: the request rides on rt.heap_grow, whose n argument carries a flag bit (2^61, below
the heap profile's 2^62) and whose heap argument is the magazine, because a new rt.* name needs the seed's
compiler to know it (gotchas/seed-older-than-intrinsics.md: a runtime function calling a name the seed
lacks cannot be built by it) and a seed bump needs the owner. Follow-up: replace it with a named
intrinsic (rt.heap_scavenge, with its wasm lowering) after the next seed cut, so the flag does not become
permanent by accident; the comments at alloc.xo free_mt and cabi.xo heap_grow say the same.

## Per class central locks (2026-10-27)

The cut path (a) cost about +3% wall on the cold self build (1.031, CI 1.027 to
1.039) while the layout alone was free (1.003). Batching the cut under one lock hold
per side did not help (1.038), and a CPU profile put the extra time in
`alloc.alloc` and `alloc_pop` (cold reads of reused blocks), not in lock waits; the
cost was the single `heapos.lock` being held across the cut by a growing thread while
every other thread's refill and flush waited for it.

Change: one lock word per size class instead of the one heap lock (as Go's mcentral):
`cen + CLOCK (2048) + (c << 6)`, taken by `alloc.clock(h, cen, c)` (a CAS fast path;
contended, `rt.heap_grow(h, LOCK_SLOW + c)` parks, counted in W_LOCKS) and released
by `cunlock` (swap to 0; a 2 wakes through `LOCK_WAKE + c`). The central table page
is 8 KiB. `cut_for` takes the donor class's lock to detach up to `XO_CUT` KiB
(default 1024, 0 = off) of magazines, cuts them without a lock, and splices the
result under the requested class's lock.

Measured (20 rounds, quiet, cold self build of compiler/xo, A main, B this): wall
1.001 (0.998 to 1.002), RSS within noise. The cut's 3% is gone and its footprint
saving (-0.75 GB peak) stays.

## Gate (for every step)

The fixed point (scripts/bootstrap-xo.sh s2 and s3 identical), a self build's peak RSS and peak
memory footprint before and after (`/usr/bin/time -l xo build ./compiler/xo`, a cold private cache,
heavy.sh bulk), `xo bench --ab` on the alloc and concurrency benches (bench/perf alloc, maps, text,
http and the scheduler's) in a quiet window, because the madvise of (c) and the splitting of (a) sit
on the central path every thread's refill takes, and XO_HEAPSTATS' free list report unchanged in
shape. The allocator's no allocation rule (check_no_alloc) holds for alloc.xo and heapos.xo's
changes.

## Consequences

xo's own peak falls from 10.4 GB toward the live set plus the classes' working set, which is what
bounds how many builds the Mac runs at once (decision heavy.sh HEAVY_MAX_GB). Memory a process frees
returns to the OS, as Go's does, with the cost of refaulting it on reuse.

## Alternatives

- Per thread caches that take from the central lists of any class: the same as (a) with the
  splitting done at the central lists' lock; costs a lock hold per cut.
- Coalescing free blocks of a class into the next class up (the buddy system): needs the per chunk
  or per span structure the allocator does not have; (a) covers the direction that matters (large
  freed, small wanted).
- Fewer allocations in emit (arenas per function): shrinks the churn itself (33 GB in 17 s) but is a
  compiler change per pass, not a heap change; kept as the next item after step 0 shows which classes
  the churn is in.
- Lowering MAG_BYTES so fewer blocks sit in magazines: leaves the memory in lists of the wrong class.

## Follow-up (decision 0425, 2026-10-30)

Page blocks moved the 16 KiB and larger classes' headers and links into a per-chunk table, so (c)
scavenges every page of a free block of those classes, not only the inner ones (the 32 KiB class
gave back one page in two, now two). The 16 KiB class could be scavenged too (alloc.xo
SCAVENGE_FROM 68: 0.8 GB less footprint on the self build for 0.8% of its wall) but stays at 69 by
the lead's rule, and (a)'s cut stops at the 8 KiB class (heapos.xo CUT_MAX_CLASS 67, was 68): a 16
KiB magazine is no longer carved into 8 KiB blocks, since its blocks carry no prefix to carve into.
