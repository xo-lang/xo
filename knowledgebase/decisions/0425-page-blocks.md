# 0425: Page blocks: the 16 KiB and larger size classes keep their class and links in a per-chunk table

Status: Accepted (xo-lang-6, 2026-10-30; the owner's go on the plan 2026-10-29)

## Context

The native allocator (std/native/llrt/alloc.xo) puts a block's size class in the 8 bytes
before the block and its free list link in the block's first word, so a free 16 KiB or 32 KiB
block (classes 68 and 69) keeps one resident page for its prefix and links, and heapos.xo
scavenge (decision 0383) gives back only its inner whole pages. The self build's footprint
holds about 0.8 GB of such pages (probe, 2026-10-29).

## Decision

Blocks of class 68 and up (16 KiB to 256 KiB, PAGE_CLASS in std/native/llrt/alloc.xo) are page
blocks: page aligned, with no prefix, carved from page chunks (heapos.xo pchunk: CHUNK bytes,
4 MiB, mapped aligned to their size by map_aligned, which maps twice the size and gives the slack
back; Windows reserves twice and commits the aligned part). The chunk's first page is a table with
one word per 16 KiB page of the chunk: the block's class at PT_CLASS (with PSAMPLED, bit 7, when
the heap profile sampled it), its free list link at PT_LINK and its magazine link at PT_CLINK,
indexed by the block's first page; alloc.xo's hot paths (alloc, free, free_push, resize) test
`c >= PAGE_CLASS` or the address's alignment inline and send the page aligned case to a cold
function by a tail call (free_aligned, free_push_aligned, resize_aligned, in ssa.cold_fn), so a
prefixed block's free and resize stay leaves with no frame (the first version's helpers and inline
page paths cost the alloc bench's tasks workload 14% of its instructions). Every chunk, page or not, is 4 MiB aligned (map_aligned), and a
page chunk's first word is PAGE_MAGIC: a block's kind is told by its address and that word (alloc.xo
is_page): a page block is a multiple of 16 KiB in a chunk whose first word is the magic; a big block
is 16 past a page; a prefixed block is 8 aligned (its stride is size + 8), so one in 2048 is page
aligned too and is told apart by its chunk's first word, the first block's class. The list helpers (lnext, lset, cnext, cset) take whether the list's blocks
are page blocks; every pop, push, refill, recount, resize and scavenge goes through them.

1. Each heap record has its own page bump (PCUR, PEND at 1888 and 1896): alloc carves a page block
   from it, and cabi.xo heap_grow maps a new page chunk when it runs out (before 0383's cut, which
   now covers classes to 67 only: CUT_MAX_CLASS 67).
2. free, free_push and free_mt read a page block's class from the table; a sampled one is first
   forgotten in the heap profile through rt.heap_grow(p, PFREED) (2^58, the next free flag after
   LOCK_WAKE), as heap_big_release does for a prefixed block's negative header.
3. resize keeps a page block in place when its class serves the new size, else the caller moves
   it; a prefixed block never grows into a page class in place (its pages would not be aligned).
4. heapos.xo scavenge (0383 c) gives back every page of a page block, not only its inner ones,
   and walks the table links. SCAVENGE_FROM stays 69: the 16 KiB class could be scavenged whole now,
   and was in the first measurement, for 0.8 GB more footprint at 0.8% more wall (the table below);
   the owner's 7 GB goal holds without it, and the lead's rule (2026-10-30) was the cheapest setting
   under 7 GB.
5. Page blocks are off (PAGES word 1904 of every heap record, heapos.pages_on) in a sanitizer or
   --race build: their big blocks come from the C library's malloc, whose 16 byte alignment can
   put a big block's data on a page boundary and in no chunk, so is_page's chunk word read would
   fault. Those builds keep the prefixed layout for every class.
6. heapprof.xo sample marks a page block in the table (alloc.page_sample) instead of negating a
   header it does not have.

## Consequences

- Self build, cache off (A the base commit's xo and std, B this change, 2026-10-30 02:14, a quiet
  window): peak memory footprint 7864 to 6025 MB (-1.84 GB, -23%), max RSS 10.16 to 10.12 GB, mapped
  (heap/peak) 10212/11280 to 10367/11402 MB: the pages are given back, the chunks stay mapped. Free at
  exit (XO_HEAPSTATS): class 68 735 MB, 69 1236, 70 1444, 71 750, 72 1261, all of it scavenged.
- The settings measured (self build, cache off, one quiet window each; --ab 6 rounds; the no
  scavenge row's --ab ran loaded, 5 of 6 rounds, and is not usable):

  | setting | footprint MB | sys s | --ab wall against the row above it |
  |---|---|---|---|
  | A: the base (prefixed blocks, scavenge 69 and up, inner pages) | 7864 | 6.5 | |
  | page blocks, scavenge 68 and up (whole blocks) | 6025 | 9.6 | 1.033 (1.030 to 1.047) against A; CPU 1.049, instructions 1.024, cycles 1.051 |
  | page blocks, scavenge 69 and up (SCAVENGE_FROM 69, chosen) | 6816 | 7.0 | 0.992 (0.984 to 1.000) against the row above; CPU 0.987 |
  | page blocks, no scavenge | 10036 | 4.3 | (worse than A: nothing given back) |

  The chosen setting against A, second window (03:01): footprint 7688 to 6855 MB (-0.83 GB, -11%;
  the base's footprint was 7864 in the first window), chunks 1986 to 2070 (the cut no longer carves
  16 KiB magazines), sys 6.5 to 6.5 s; --ab 6 rounds (2 loaded, kept): wall 1.020 (0.999 to 1.030,
  within noise), CPU 1.024 (1.011 to 1.033), instructions 1.019, cycles 1.029, max RSS within noise.
- Benches on the chosen setting, final tree (A the base runtime, B this one; the Mac, 8 rounds,
  2026-10-30 03:51; wall and cycles within noise everywhere): alloc tasks instructions 1.056 (1.038
  to 1.063; the page aligned test and the branch free count on every free_push, the page class
  test on every alloc), mixed 1.011, strings 1.013, grow 1.010. On the linux/arm64 offload host (ab-host.sh, 2026-10-30 06:00, wall only: GNU time counts no instructions): the self build (6 rounds, 1 warmup, --keep-loaded) wall 0.998 (0.988 to 1.017, within noise; CPU 0.968, 0.965 to 0.994, better), max RSS 0.997 (8489 to 8468 MB, within noise); alloc tasks/mixed/strings/grow wall 1.005/0.984/1.017/0.959 (grow better, 0.946 to 0.966; the rest within noise); json small wall 1.017 (0.981 to 1.038, within noise). Max RSS of the small programs on Linux: alloc tasks 45 to 77 MB (1.70), mixed 19.4 to 21.4 (1.11), strings 1.00, grow 7.0 to 9.1 (1.30), json small 11.5 to 13.5 (1.18): a page chunk is 4 MiB and every heap that frees a 16 KiB block keeps it (SCAVENGE_FROM 69), where the cut carved 16 KiB magazines from blocks it had anyway; XO_HEAPSTATS=1 on alloc tasks on the host (12 threads): chunks 12 to 22 (48 to 88 MB mapped), RSS 45.4 to 70.2 MB, free lists 2 MB both: every heap that takes a page block gets a 4 MiB page chunk of its own beside its chunk (PCUR and PEND are per heap record), and the resident part of ten such chunks is the growth; a follow-up (followups.md: page blocks carved from the heap's own chunk, or one page chunk per process) can give it back; the owner's goal, the self build's peak, is unaffected. The self build's Linux max RSS is within noise where the Mac's footprint fell 11%: scavenge frees at once on both (Linux MADV_DONTNEED, Darwin MADV_FREE_REUSABLE), but a page is 16 KiB on the Mac and 4 KiB on Linux, so the header page a freed prefixed big block kept resident cost four times as much there (the saving is per freed block, not per byte). The final tree's footprint on the Mac
  under load (two other gates running, 04:05; the self build took 82 s against 46 s for A): A 8036
  MB, B 7483 MB (-7%, chunks 1981 to 2069); the quiet window pairs above (A 7864 and 7688, B 6816
  and 6855) are the ones for the owner's goal: a slower build keeps more parts in flight, so the
  footprint of a loaded machine is not the one to compare. The figures of this decision are the quiet pairs (the lead agreed, 2026-10-30 04:40); the machine stayed loaded (1 minute load 12 to 28) through the morning, so no third quiet pair was taken.
- The hot paths and the inliner (XO_INLINE_REPORT on bench/perf/alloc, limits 40 instructions and
  12 blocks for a function inlined at every site, 500 instructions for one with a single site): the
  first cut sent a page aligned block out of free_push through a call (free_push_aligned) and
  resize's prefixed path to a second function shared with resize_aligned; free_push's cost went 34
  to 78 (a call makes the function a non leaf in inline_walk's count even in tail position, so its
  constants count: followups.md) and it became a call in obj.rl_free_n_slow, 14 instructions a free
  (alloc tasks 1.145 instructions, callgrind); resize's prefixed path had two sites at 316 and
  stayed a call in obj.hresize (alloc mixed 1.054). Now free_push returns false for a page aligned
  block (free_mt tells page blocks apart) and counts a free by adding COUNT (0 or 1) without a
  branch, 35 instructions and 10 blocks (34 and 10 before this decision); resize keeps its prefixed body and tail calls
  resize_aligned (ssa.cold_fn, with free_aligned), which moves the one prefixed block in 2048 that
  is page aligned instead of resizing it in place.
- The first measurement's alloc and json benches (scavenge 68 and up; wall within noise everywhere):
  alloc mixed instructions 1.012, cycles 1.013; grow 1.010; strings 1.026; json small 1.009, cycles
  1.018; alloc tasks (threads: magazines, the central lists) instructions 1.15, cycles 1.46 with a
  bimodal wall (0.43, interval 0.41 to 1.16): the threaded page block paths (a class word and a link
  in the chunk's shared table, written by whichever thread frees the block) are the cost to watch;
  (The rerun on the chosen setting is the final tree row above.)

- Every chunk is mapped twice its size and the slack given back (three system calls instead of
  one per 4 MiB chunk; a sanitizer build keeps the 8 MiB from the C library).
- A chunk of 4 MiB holds 255 blocks of 16 KiB or 127 of 32 KiB and so on, plus the table page:
  0.4% of the class's memory for the table, against the page a prefixed block kept resident.
- Every heap record maps its own page chunk on its first page class allocation (8 MiB of address
  space while the slack is being given back, 4 MiB after); only the touched pages are resident.
- A libc or race build (pages_on false) is the old layout: its footprint gains nothing.
- 0383's cut no longer carves 8 KiB blocks out of 16 KiB magazines (CUT_MAX_CLASS 67): the
  donors stop at the last prefixed class.

## Alternatives

- A prefix out of the block, class in a per-chunk table, for every class (the owner's phrasing):
  the same table for small blocks costs a lookup on every free of the hottest classes and moves
  the 8 byte header the small paths rely on (data at 8 mod 16); the gain is only in the page
  classes, whose free blocks are the resident pages.
- Scavenging the prefix page too (madvise the whole block and rewrite the prefix on reuse): the
  free list link lives in the block, so a popped block's first page faults back in anyway and the
  prefix write dirties it before the user does; and the central list walk would touch every
  block's page.
- Go's layout (spans with out-of-line metadata for every size): a far larger change to a
  working allocator; this takes its idea for the classes where it pays.
