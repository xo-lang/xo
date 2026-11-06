# 0506: size classes above 1 KiB, four per doubling

Status: Accepted (owner, 2026-11-05, through the lead) on its own bar: it ships only if the --ab
self build shows lower max RSS and wall time not worse. Proposed (Xo Lang 3, 2026-11-04, at the lead's request; the class
change and its --ab are Xo Lang 2's, who owns the heap code and 0505)

## Context

heapos.xo `class_bytes`: 16 byte classes to 1 KiB, then one class per
doubling: 2040, 4088, 8184, 16376, 32760, 65528, 131064 and 262136 bytes
after the 8 byte prefix. A buffer whose payload is a power of two (the usual
size of a read buffer, a body buffer or a page) plus its header does not
fit the class below it and takes the next, so it maps twice its size.
Go's classes are about four per doubling.

Measured (bench/results/size-class-waste-2026-11-04.md, bench/perf/sizewaste,
linux/arm64 host 2, about 195 MB of payload held per row, RSS growth over
payload; Go is 1.00 to 1.08 in every row):

| payload | Xo | Go |
|---|---|---|
| 8, 16, 32, 64 KiB | 2.01, 2.02, 2.03, 2.06 | 1.01 to 1.03 |
| 128 KiB | 2.14 | 1.00 |
| 12, 24, 48 KiB | 1.34, 1.35, 1.36 | 1.00 to 1.02 |
| 16,000 bytes | 1.04 | 1.03 |

Xo's own build (xo building compiler/xo on host 2): maximum RSS 3.86 GB, and
at exit the classes above 1 KiB hold 1.85 GB of the 3.55 GB of retained free
blocks (context only: free blocks are not the doubling's waste; that needs
live requests per class, still open). std buffers of a power of two: http.xo's 32,768 byte
pass-through buffer (a 64 KiB class), linux.xo's 16,384 byte buffers (a
32 KiB class), pg.xo's 65,536 + 1,024 byte one (a 128 KiB class).

## Decision

The live request census (bench/results/size-class-waste-2026-11-04.md "Live bytes by request size", the self build at inline.verify, 2,097 MB in use): requests over 1 KiB hold 68% of the live bytes, 4 KiB to 256 KiB hold 47.5%. Shape (Xo Lang 2's review): below 16 KiB
classes at about 1.25x (1280, 1536, 1792, 2048, 2560 ... in 256 B, 512 B and
1 KiB granules, each a step minus the 8 byte prefix); from 16 KiB the page
classes (runs of 16 KiB pages) in multiples of 16 KiB: 32, 48, 64, 80, 96,
112, 128, 160, 192, 224, 256 KiB. The 16 byte path to 1 KiB stays a shift;
a small table serves requests above 1 KiB. Bar: --ab self (max RSS, wall),
bench/perf/alloc small and mixed, darwin footprint. A cheap first step
regardless: size std buffers just under a class (http.xo 32,768 to 32,736).

## Consequences

- A request between classes wastes at most about 20% (one 1.25x step; at most
  one 16 KiB page from 16 KiB up for sizes up to 128 KiB, then 32 KiB steps),
  against up to 50% now. On the self build's census (requests over 1 KiB hold
  1,424 MB of 2,097 MB live at inline.verify) the mean waste falls from about
  25% of those bytes to about 10%: roughly 200 MB of that point's live bytes
  (an estimate from the request histogram, to be confirmed by the `--ab` self
  run's max RSS).
- Measured with the per-class counter (`XO_HEAPSTATS=waste`, 051dc2417, Xo
  Lang 2; a cold self build of xo on linux/arm64 host 2, sums over the whole
  run, not live bytes): above 1 KiB the allocator was asked for
  201,021,794,503 bytes and handed 240,316,119,040, 1.195 bytes per byte
  asked (the handed figure counts the 8 byte prefix); per class the ratio
  runs 1.134 to 1.268 (classes 65 to 72), and page rounded big blocks 1.019.
  Four classes per doubling can therefore recover at most about 16% of the
  bytes handed above 1 KiB (1 - 1/1.195), less the finer classes' own
  rounding. This replaces the live-bytes estimate above as the ceiling;
  whether it shows in max RSS is the `--ab` self run's job (the counter reads
  sizes asked over the whole run, so it equals live waste only if lifetimes
  do not depend on size within a class). Write-up:
  knowledgebase/infrastructure/memory-footprint.md "Size classes above 1 KiB".
- More classes mean more partly used runs and a larger free list set: the
  retained free blocks at exit may grow (3,553 MB on the self build now); the
  `--ab` self run and `XO_HEAPSTATS=1` decide.
- A request above 1 KiB pays a table lookup instead of a shift: the bar is
  bench/perf/alloc small and mixed within noise.
- No language or ABI change: block sizes are the allocator's. Lang 2 owns
  the change; this ADR stays Proposed until the owner accepts it.


## Implementation (2026-11-05, Xo Lang 2)

- Stage A (5c782c1a2): std's power of two buffers were moved just under a class. These were
  the TLS and getdents reads (16,352), http's pass-through (32,736) and pg's COPY builder
  (65,504). Each had taken the class twice its size.
- Stage B: classes 65 to 76 are blocks of 1280, 1536, 1792 and 2048 bytes, then four a
  doubling up to 8192. Classes 77 to 81 are 16 KiB to 256 KiB page blocks, powers of two as
  before (decision 0425), so CLASSES is 82 and PAGE_CLASS is 77.
  - alloc.xo computes the class in constant time: the block's exponent by comparisons,
    since rt.clz is not among the heap module's intrinsics (decision 0207), then the
    quarter from two bits.
  - class_of and class_size serve heapos.xo, obj.xo and entry.xo, which had five copies of
    the doubling loop.
  - The per class arrays (free list, length, spare magazine, magazine size) moved to the
    heap record's end. Every other word keeps its offset.
  - cut_for's donors stay below the page class (CUT_MAX_CLASS 76). Above 1 KiB a class takes
    donors only from the classes 4, 8, ... after it, whose blocks are exactly 2, 4 or 8 of its
    own. cut_for drops the part of a donor block that does not fill a whole block, and with
    four classes a doubling most pairs are not exact: the first build, which let every
    larger class donate, held 282 MB of the self build on no list (XO_HEAPSTATS on the
    linux/arm64 host: 815 MB of the chunks neither live nor free, against 533 MB on main),
    and its max RSS was 10% above main (gotchas/size-class-cut-tail-leak.md).
  - Four a doubling all the way to 16 KiB, with exact cuts, still peaked 3.5% above main
    (4,346,076 to 4,498,204 KB) though it mapped fewer chunks. Putting 8 to 16 KiB back in
    the 16 KiB page class brought the peak below main (variant D, shipped). Adding back
    only a 10 KiB class, so that an 8 KiB buffer takes 1.26 times its payload and not
    2.01 (variant E), put the peak 2.4% above main again over 5 unloaded rounds: in the
    self build, 8 to 16 KiB requests cost less at peak as 16 KiB page blocks than as
    smaller prefixed blocks. So 8 KiB buffers stay at 2.01 (std sizes its own just under
    a class, stage A), and 8 KiB to 16 KiB is where a later stage would look (why the page
    blocks peak lower is not measured yet).
  - Measured (bench/results/size-classes-stage-b-2026-11-05.md), xo bench --ab self on the
    linux/arm64 host against main a27137842, 5 rounds: max RSS 0.986 (4,341,468 to
    4,280,256 KB), wall 0.994, cpu 0.993 (4 of 5 rounds loaded by other jobs; a 3 round
    run: max RSS 0.971, wall 0.993).
  - Rejected, each by the same --ab self against a27137842:
    - four a doubling to 16 KiB (classes 65 to 80), every larger class a cut donor: 3
      rounds, max RSS 1.101 (4,337,788 to 4,777,320 KB), wall 0.985;
    - the same with exact cuts only: 3 rounds, max RSS 1.035 (4,346,076 to 4,498,204 KB),
      wall 0.992;
    - E, the shipped layout plus a 10 KiB class: 5 rounds, none loaded, max RSS 1.024
      (4,307,140 to 4,409,308 KB), wall 0.981 within noise.
  - bench/perf/alloc on the linux/arm64 host: small wall 1.015 worse (31.15 to 31.61 ms;
    classes under 1 KiB are unchanged, so likely the per class words' move to the record's
    end), mixed max RSS 0.805 and wall 0.988 better. Darwin footprint: xo bench --ab self on the Mac (darwin/arm64), caches off, 3 rounds: max RSS 1.004 within noise (4,473,024 to 4,489,728 KB), wall 1.000, cpu 0.995.
  - Tried after D and not kept (2026-11-05, Xo Lang 2): the free lists and their lengths
    (FREE_LISTS, NFREE) back in the record's first pages, at the words the pre-stage-B
    arrays used (80 and 760), with MAG and MAG_CAP at the end, to test whether the small
    regression above came from the move. It did not: --ab against its parent 0950f781d,
    bench/perf/alloc small 20 rounds on the 18 CPU linux/arm64 host 0.999 (31.33 to 31.31
    ms, none loaded) and 10 rounds on the 12 CPU host 0.990 within noise; mixed 1.003 wall,
    1.000 max RSS; self 5 rounds (4 loaded) wall 1.002, max RSS 0.993 within noise. The
    3 round 1.015 above is so far unexplained; a 20 round small run of a27137842 against
    0950f781d is the next check.

## Alternatives

- Keep the power-of-two classes and size std buffers just under a class
  (16,000 instead of 16,384): fixes std's own buffers, not the user's.
- Round large blocks to pages (Go's way above 32 KiB): a different allocator
  path for sizes now served by classes.
