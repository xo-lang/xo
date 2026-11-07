# 0533: Return the pages of long free class blocks on Linux (a time based scavenger)

Status: Proposed (Xo Lang 2, 2026-11-06; direction approved by the owner 2026-11-06: Go-style, a background pass; this text reported for acceptance)

## Context

Two measurements of stability/notes on linux/arm64 point at one cause: pages of
free heap blocks that a long running Linux process never gives back.

1. The soak (decision 0520 check 1, 500 req/s, leakcheck 0 live blocks):
   - RSS outside huge pages grows 8 to 10 MB/h: +8,247 KiB/h with THP on, AnonHugePages flat at 2,048 KiB;
     idle30 +9,994 KiB/h to 31.7 MB.
   - Free lists 9.5 MB at 20 min, 11.2 MB at 1 h. Chunks 14 (56 MB mapped) at both, so the growth is touched
     pages of free blocks inside chunks already mapped, not new mappings.
   - 6 h (long6h, dc72da981, XO_HEAPSTATS=1, 500 req/s, 10.79M requests, 0 errors): leakcheck 0 live at exit; RSS 25.1 to 41.7 MB, slope 3,540 KiB/h, of which outside huge pages 1,280 KiB/h (8 to 10 MB/h at 1 h, so the growth slows but does not stop within 6 h) and AnonHugePages 6 to 14 MB; heap_live 935 to 1,860 blocks while the client held connections rose 24 to 83 (fds flat over the second half), so the live growth follows the harness's connections, not a leak; p99 1,332 to 611 us; verdict DRIFT.
2. The load sweep (harness load, map store, 2 rounds, host 2, 2026-11-06):
   - RSS rose about 90 KiB per extra busy connection, flat from 128 to 256 workers. Go's twin is flat at
     53 to 57 MB.
   - Fiber stacks: about 9 KiB resident each (3.0 MB at 351 fibers).
   - heap_chunks 18 (72 MB) at 8 workers, 38 (152 MB) at 128 and at 256.
   - Free list bytes at exit: 22 MB at 8 workers, 53 MB at 256, +32.7 MB.
   - By class, 8 to 256 workers:

     | class | block size | free list growth |
     |---|---|---|
     | 79 | 64 KiB | +6.2 MB |
     | 80 | 128 KiB | +3.9 MB |
     | 81 | 256 KiB | +3.7 MB |
     | 78 | 32 KiB | +3.2 MB |
     | 77 | 16 KiB | +1.4 MB |
     | 8 and 9 | 128 B and 144 B | +1.8 MB |

   - Live heap at busy: 1.5k to 6k blocks. The memory is in-flight request working memory times concurrency,
     kept on the free lists after the requests end.

Today (alloc.xo SCAVENGE_FROM 78 (32 KiB), SCAVENGE_KEEP 4, decisions 0383, 0425 and 0448):
- Only on a free that completes a magazine, and only for a class whose central list already holds 4
  magazines: that magazine's pages go back (heapos.scavenge: MADV_DONTNEED on Linux, at once;
  MADV_FREE_REUSABLE on darwin). Linux does this for 32 KiB and up.
- The 16 KiB class (scavenge_pages()) is darwin only: on Linux the refaults cost the warm build's wall.
- Blocks on the threads' own free lists and current magazines, and the 4 kept central magazines per
  class, are never given back, and nothing is given back without a free that completes a magazine.
- Classes under 16 KiB are never scavenged.
- Caution for the numbers above: HEAPSTATS's free list bytes count address space, so a scavenged block
  still counts there; the RSS rows (mem.txt) are the measure of the change.
- The self build is the constraint: it never idles, and any advice on its alloc path costs wall time
  (stage1-v2: 3 to 4.7% on Linux with MADV_DONTNEED).

Go: a background scavenger returns pages of free spans to keep retained memory about 10% over the heap
goal. It uses MADV_DONTNEED on Linux by default since Go 1.16, so RSS reflects it.

## Decision (direction approved by the owner, 2026-11-06: Go-style, a background timer)

What exists (decisions 0448 and 0505): debug.mem_trim gives back, on Linux too, the pages of the
calling thread's free page blocks (heapos.trim_thread with Linux's page size) and of every central
magazine (trim_central), each block once (PADVISED). The idle path already calls trim_thread after
FIBER_TRIM_US (pool.xo go_idle, HEAP_TRIM), but there with no page size on Linux, so it gives nothing
back: Linux MADV_DONTNEED refaults cost the warm build's wall at every phase (ab-host stage1-v1/v2,
2026-10-31), because a build's workers do idle 3 s between its phases and then fault the same pages
back in. The missing piece is pacing, not mechanism.

Decided: an age rule instead of an idle rule, as Go's scavenger returns only what stayed unused.
- Two marks per page block in its chunk's table word: PSEEN (alloc.xo, new, 512) and PADVISED (256,
  exists). A pass over a list of free page blocks advises a block already PSEEN and not PADVISED
  (heapos.xo advise: MADV_DONTNEED on Linux when the page size covers the block exactly, the rule
  trim_list already had; MADV_FREE_REUSABLE on darwin) and marks the others PSEEN. A block's free
  clears both bits (alloc.xo page_class), so only a block that stayed free from one pass to the next
  goes back.
- The passes run from pool.xo go_idle's entry (age_check), at most once per SCAV_EVERY_NS (30 s) per
  worker: the worker ages its own heap record (its free lists and spare magazines, no lock: a record
  is its thread's alone), and the first worker past the process's period (the pool record's P_AGE_AT_I word, the
  last central pass's time, claimed by a compare and swap; static words 60 to 99 are all taken) also walks the central lists' magazines of the
  page classes under each class's lock (heapos.xo age). An idle point, not a timer thread: under
  constant load workers still go idle between tasks, and the monitor thread (mon_main) sleeps when the
  pool is quiet, so it cannot serve. The check is one clock read (vDSO) per go_idle.
- A quiet process: a worker idles once and sleeps until woken, so a pass at its idle entry alone
  would mark blocks and never return to give them back. A worker whose pass marked blocks
  (W_AGE_MORE) sleeps at most a period instead (its futex wait timed, or its wait in the poller
  bounded by SCAV_EVERY_NS) and runs the next pass when it times out; a pass that marks nothing
  clears the flag, so an idle process wakes each worker at most twice, then sleeps as before.
- So a block goes back 30 to 60 s after its last free; a build's churn between phases (3 s idles)
  never reaches the second mark, which is what made the idle trim's refaults cost decision 0448's wall.
- Linux and darwin alike; darwin keeps its lazy MADV_FREE_REUSABLE on the free path as today, and
  Windows and wasi advise nothing. Sanitizer and --race builds skip it (pages_on off).
- Out of scope: classes under 16 KiB (prefixed blocks, the +1.8 MB of 128 and 144 B), the cached
  1 to 2 MiB blocks (0505 finding 2), and a program with no worker threads (no go_idle; debug.mem_trim
  stays its tool).

## Consequences

- A free page block goes back 30 to 60 s after its last free, on Linux too; a request after that
  faults its pages back in (a few µs per 16 KiB). Blocks a busy program reuses within a period
  keep their pages, so the warm build's phase churn (3 s idles) pays nothing.
- An idle worker whose pass marked blocks wakes once more a period later (W_AGE_MORE); then it
  sleeps as before. The cost on the idle path is one vDSO clock read per go_idle.
- Measured (2026-11-06):
  - probe (64 tasks holding 24 blocks of 60 KB each, 3 rounds, then idle; linux/amd64 offload host,
    4 KiB pages): RSS 27.0 MB after the load, 27.0 MB at 40 s, 24.1 MB at 75 s; the base std stayed
    at 24.6 MB. Most 64 KiB magazines already go back on the free path (SCAVENGE_KEEP); the age
    passes add the kept magazines, the 16 KiB class and the workers' own lists.
  - stability/notes RSS idle 75 s after load (host 1, linux/arm64, 2 rounds each, busy -> idle KiB, base 1d8571422, new 7266bf502):
    - THP as the host has it (enabled=always, khugepaged max_ptes_none=511), AnonHugePages in brackets where recorded: sat8 base 18160 [0] -> 26768 [10240], 19644 -> 19684; new 15828 [0] -> 15440 [0], 25372 [2048] -> 24288 [2048]; sat128 base 35908 -> 35912, 34996 -> 41076; new 36176 -> 34800, 32332 -> 30912; sat256 base 38688 -> 38692, 38784 -> 38800; new 38712 -> 35864, 37968 -> 39180. The idle rises, in both builds, are khugepaged collapsing partly touched 2 MiB ranges into huge pages (base sat8 round 1: +8.6 MB RSS with +10 MB AnonHugePages), not the age pass.
    - XO_THP=off (decision 0454, MADV_NOHUGEPAGE on every heap mapping; AnonHugePages 0 throughout): sat8 base 19024 -> 19028, 16640 -> 16664; new 16324 -> 16072, 22396 -> 20860; sat128 base 37204 -> 36924, 34280 -> 34292; new 35844 -> 33792, 34088 -> 32220. Without khugepaged the base gives nothing back at idle (-280 to +24 KiB) and the age pass returns 252 to 2,052 KiB per run.
    - Go has the same knob, GODEBUG=disablethp=1 (go1.26.1 runtime/mem_linux.go sysMapOS: MADV_NOHUGEPAGE per heap mapping), a compatibility setting since Go 1.21 stopped working around the kernel's THP default (go.dev/issue/64332), with operators told to tune THP on the host (gc-guide, Linux transparent huge pages). Xo's XO_THP=off already does this, so 0533 adds no switch.
  - self build --ab (3 rounds, linux/arm64 host 1, 2026-11-06, ab-host 1d8571422 against 7266bf502): wall 1.002 (0.943 to 1.002) and cpu 1.000, within noise; max RSS 0.990 (0.982 to 0.997), better. The run's load of 6 to 7 is the build's own (224 s of CPU in 33 s of wall); the host's baseline before it was 0.94.
  - the 6 h soak's RSS before the change: 6 h (long6h, dc72da981, XO_HEAPSTATS=1, 500 req/s, 10.79M requests, 0 errors): leakcheck 0 live at exit; RSS 25.1 to 41.7 MB, slope 3,540 KiB/h, of which outside huge pages 1,280 KiB/h (8 to 10 MB/h at 1 h, so the growth slows but does not stop within 6 h) and AnonHugePages 6 to 14 MB; heap_live 935 to 1,860 blocks while the client held connections rose 24 to 83 (fds flat over the second half), so the live growth follows the harness's connections, not a leak; p99 1,332 to 611 us; verdict DRIFT; after: the paired soak below.
  - paired soak (2026-11-07, host 2, 1 h legs in the order base, new, new, base, 500 req/s,
    XO_HEAPSTATS=1, harness at each revision, so `-client-idle 0`; base 1d8571422, new 7266bf502;
    paired-0533.sh). RSS only: cpu/req and p99 moved with the host's load (means 1.9 to 6.8,
    peaks 9 to 38) and the client's pool, so they support no claim either way.

    | leg | rev | RSS slope after warmup | RSS at 1 h |
    |---|---|---|---|
    | p1 | base | 16,440 KiB/h | 30.8 MB |
    | p2 | new | 12,301 KiB/h | 23.6 MB |
    | p3 | new | 11,683 KiB/h | 24.0 MB |
    | p4 | base | 12,156 KiB/h | 28.1 MB |

    New against base: slope 0.84x (11,992 against 14,298 KiB/h), RSS at 1 h 0.81x (23.8 against
    29.5 MB). Both still drift by 12 to 16 MB/h. That drift is before 0533 and is being chased as its
    own item: in a 1 h base leg (2026-11-07) RSS rose 12 MB/h while the client's sockets were flat
    and heap_live fell, so it is not the client's pool alone.
  - age_sparse (93f714153, the amendment below): the age passes skip a block whose 2 MiB range is
    under half free when khugepaged can collapse it. Rows at `-idle 75s` with THP as host 1 has it,
    busy -> idle KiB: sat8 base 17796 -> 28160, 23944 -> 29440; new 17964 -> 26264,
    26692 -> 26164 (new idle under base in both rounds);
    sat128 base 33852 -> 33856, 33356 -> 33364; new 34120 -> 31612, 40536 -> 39116.
  - sat128 at `-idle 150s` with an idle@75s sample, 4 runs per side (order base, new, new, base,
    host 1, base 1d8571422, new 93f714153):
    - THP as the host has it: 2 of 8 runs re-inflated while idle, one per side: base +16.3 MB
      (AnonHugePages 0 to 20 MiB by 75 s); new +10.0 MB between 75 s and 150 s after giving back
      2.7 MB (AnonHugePages 0 to 14 MiB). The other six fell or stayed flat (new -1.2 to -2.4 MB, base within 8 KiB).
    - XO_THP=off: 0 of 8 re-inflated, AnonHugePages 0 throughout; new gave back 0.9 to 2.9 MB by
      150 s in every run (mean 1.7 MB), base 0 to 0.9 MB (mean 0.2 MB).
- With it, heapos.xo scavenge advises a page block only when the system's pages cover it exactly
  (gotchas/madvise-rounds-over-neighbor.md): under 64 KiB pages a 32 KiB block's advice had rounded
  over its neighbor.
- Follow-up: a /debug/sched line for the bytes the age passes gave back, beside heap_chunks.

## Amendment (Xo Lang 2, 2026-11-07): released ranges and khugepaged

Cause of the idle rises: on host 1 (THP `enabled=always`, khugepaged `max_ptes_none` 511, the
kernel default, scan every 10 s) khugepaged collapses any 2 MiB aligned range of an eligible
mapping that has one resident page, filling the other 511 with zeroed memory
(gotchas/khugepaged-refills-released-pages.md). A range given back with MADV_DONTNEED stays
eligible, so a collapse can undo the age pass. The base's re-inflation shows the same collapse of
sparse ranges that were never given back.

Amended decision, both on Linux only and only while huge pages are possible (THP `always` and
the heap not XO_THP=off):
1. age_sparse (93f714153): an age pass gives back a block only when its 2 MiB range is at least
   half free, as Go's scavenger skips dense chunks (mgcscavenge.go). This keeps a lone hole out of
   a dense range, and it fixed sat8 (above).
2. MADV_NOHUGEPAGE on each range an age pass gives back, after MADV_DONTNEED. khugepaged then skips
   every 2 MiB range that overlaps it, which age_sparse alone did not ensure: the sat128 run that
   re-inflated had given back 2.7 MB first. The flag stays when the range is reused, since Linux
   has no advice back to the default (MADV_HUGEPAGE opts in, which differs from the default under
   `madvise`), so a reused range keeps 4 KiB pages.
   Before it lands, measure: the VMA count (each advised range splits its mapping until its
   neighbours carry the same flag; vm.max_map_count is 65,530 by default), the warm build's wall
   on ab-host (the TLB cost of 4 KiB pages on a reused range), and the THP-on sat128 rows again,
   expecting 0 of 8 new-side re-inflations.

Not decided here: the collapse of sparse ranges that were never given back (the base run) is
decision 0454's question (XO_THP's default). This amendment only keeps the age passes' returns
from being undone.

## Alternatives

1. Lower SCAVENGE_KEEP on Linux: on the alloc path, the cost 0383 measured.
2. Per class free list caps that return blocks to their chunk, then chunk-level page return: the most
   complete, and a larger allocator change.
3. Leave it and reduce the working memory instead: presize or stream large responses (being done
   anyway as a std change, which lowers the peak but not the retention).
4. MADV_FREE instead of MADV_DONTNEED: cheaper reuse, but RSS does not drop until the kernel is short
   of memory.
