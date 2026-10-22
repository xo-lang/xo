# LLVM -O2 with a higher unroll threshold, and strings at -O3 (2026-10-21)

The owner kept -O2 as the LLVM backend's default (nbody runs twice as
fast at -O3 but strings slowed 13% and TLS text grew 6.9%). This is the
follow-up: -O2 plus one LLVM threshold, to get nbody's -O3 speed
without -O3's cost, and why strings regressed at -O3.

Goal (from the lead): nbody near -O3's 1,160 M instructions, strings
flat, code size under +2%.

## Method

- Mac (darwin/arm64), Apple clang 21. `XO_LLVM_OPT=O2` (or O3), a
  clang wrapper (`XO_CLANG`) adding `-mllvm OPT` and `-Wl,-mllvm,OPT`
  for each option, so the threshold reaches the compile and the LTO
  link (a single module build is one `clang -O2 -flto` call; its LTO
  link inlines at threshold 225 at both levels).
  `XO_BACKEND_CACHE=off XO_CLANG_CACHE=off`.
- Instructions: `/usr/bin/time -l` "instructions retired", the least of
  3 runs (run to run spread under 0.2%). Output checked by md5 against
  plain -O2: identical in every row.
- Text: `size -m`, the `__text` section.
- Workloads: bench/perf/llvm/xo (collatz, fib, interp, nbody, sieve,
  sort, strings); text size also of bench/perf/tls (the largest
  program). xo at 7f6e6c203 (before the alloc fix below).
- TLS build times in the suite are not comparable (other heavy jobs ran
  beside it); only its text is reported.

## Sweep: nbody and strings

| setting | nbody (M) | strings (M) | nbody text |
| --- | ---: | ---: | ---: |
| -O2 | 2,307 | 1,006 | 410,124 |
| -O3 | 1,162 | 1,137 | 420,140 |
| -O2 unroll 200 | 2,306 | | 410,612 |
| -O2 unroll 250 | 1,085 | | 414,360 |
| -O2 unroll 300 | 1,160 | | 409,872 |
| -O2 unroll 350 | 1,162 | | 409,872 |
| -O2 unroll 600 | 1,160 | 1,006 | 416,844 |
| -O2 unroll 1,200 | 1,162 | 1,007 | 438,204 |
| -O2 unroll 2,400 | 1,162 | 869 | 466,888 |
| -O2 unroll 5,000 | 1,160 | 871 | 467,084 |
| -O2 inline 500 | 2,307 | 1,119 | 501,932 |
| -O2 inline 1,000 | 2,308 | 848 | 649,736 |
| -O2 unroll 2,400 + inline 500 | 1,170 | 966 | 561,236 |

nbody's gain is a step between unroll thresholds 200 and 250: its inner
loop unrolls or it does not. Inline thresholds do nothing for nbody and
cost 22% to 58% text.

## Suite: every workload, -O2 with the unroll threshold

Instructions in millions; the change against plain -O2 in parentheses.

| workload | -O2 | 250 | 300 | 400 | 500 | 600 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| collatz | 3,908 | 3,908 (0.0%) | 3,906 (-0.1%) | 3,913 (+0.1%) | 3,910 (0.0%) | 3,905 (-0.1%) |
| fib | 3,829 | 3,830 (0.0%) | 3,824 (-0.1%) | 3,826 (-0.1%) | 3,828 (0.0%) | 3,825 (-0.1%) |
| interp | 1,581 | 1,581 (0.0%) | 1,581 (0.0%) | 1,581 (0.0%) | 1,582 (+0.1%) | 1,586 (+0.3%) |
| nbody | 2,308 | 1,086 (-53.0%) | 1,160 (-49.7%) | 1,161 (-49.7%) | 1,164 (-49.6%) | 1,160 (-49.7%) |
| sieve | 1,999 | 1,998 (-0.1%) | 1,997 (-0.1%) | 2,042 (+2.1%) | 2,049 (+2.5%) | 2,044 (+2.2%) |
| sort | 1,323 | 1,319 (-0.3%) | 1,322 (-0.1%) | 1,315 (-0.6%) | 1,328 (+0.4%) | 1,315 (-0.6%) |
| strings | 1,005 | 1,005 (0.0%) | 1,008 (+0.3%) | 1,005 (0.0%) | 1,007 (+0.2%) | 1,006 (0.0%) |
| tls text | 1,578,368 | 1,600,512 (+1.4%) | 1,603,372 (+1.6%) | 1,619,028 (+2.6%) | 1,621,740 (+2.7%) | 1,632,612 (+3.4%) |

For comparison, -O3 (earlier the same day): collatz, fib, interp and
sieve within 0.1% of -O2, sort -0.8%, strings +13% (fixed below), tls
text +6.9%.

Thresholds 400 to 600 miss the size goal on tls and slow sieve 2.1 to
2.5%. 250 and 300 meet every goal.

## Recommendation

-O2 with `-mllvm -unroll-threshold=300` (compile and LTO link) as the
LLVM backend's default: nbody at -O3's count, every other workload
within 0.3%, tls text +1.6%. 250 gives nbody 7% more but sits next to
the step at 200, where a slightly larger loop body loses the whole gain;
300 leaves margin. Changing the default is the owner's decision (it
needs an ADR); compiler/xo split.xo builds the flags.

## Why strings was slower at -O3

Not the program's own code (`entry.main` the same size at both levels)
and not background threads (the count is the same run to run). A
callgrind of the two builds in Docker (linux/arm64, Debian's clang 19;
the LLVM backend builds only for the host, so a linux/arm64 xo cross
built on the Mac builds the program in the container) reproduced it,
876 M at -O2 against 1,007 M at -O3, and put it in one function:

| function (self) | -O2 | -O3 |
| --- | ---: | ---: |
| alloc.alloc | 80.0 M | 241.8 M |
| everything else | 796.5 M | 765.1 M |

Per instruction (`--dump-instr=yes`): alloc.alloc ran the same prologue
at both levels, plus at -O3 a branch free 13 step long division on
every call, about 80 instructions. It is the magazine count
`MAG_BYTES / size` of alloc.refill_alloc (the magazine take, only
reached when a class's free list is empty). refill_alloc was inlined
into alloc through its tail call at both levels; at -O3 LLVM unrolled
the division and hoisted it above the free list test.

Fix (d3d212e7a): the division is its own function, alloc.mag_count,
and it and refill_alloc are in ssa.cold_fn (noinline cold in LLVM, not
inlined by the arm64 inliner). strings against the same xo with
refill_alloc renamed (so the cold list misses it):

| build | before | after |
| --- | ---: | ---: |
| LLVM -O2 | 1,005 M | 945 M (-6.0%) |
| LLVM -O3 | 1,137 M | 933 M (-17.9%) |
| native arm64 | 844.9 M | 844.3 M |

-O3 is now 1.3% below -O2 on strings instead of 13% above. Gotcha:
knowledgebase/gotchas/llvm-o3-hoists-inlined-slow-path.md.

## Build time of xo itself with 0333's default (2026-10-22)

xo's own LLVM build (compiler/xo, `xo build --backend=llvm`, every cache
off: XO_BACKEND_CACHE, XO_CLANG_CACHE and XO_BIN_CACHE), by the xo of
a32988ede (0333: -O2 with -mllvm -unroll-threshold=300), against the
same build with the flag stripped by a clang wrapper (the -O2 before
0333; the control: nbody built through the wrapper back at 2,318 M
instructions, the pre-0333 figure). One run each, in a heavy.sh bulk
slot, on a Mac loaded by other agents all night: user CPU is the
figure; wall time is not (the machine's load differed between runs).

| build | user CPU | wall | max RSS | __text |
| --- | ---: | ---: | ---: | ---: |
| -O2 (before 0333) | 1,924 s | 3,489 s | 7.7 G | 2,395,012 |
| -O2, unroll 300 (0333) | 2,170 s | 4,495 s | 5.8 G | 2,420,796 |
| change | +12.8% | | | +1.1% |

The -O3 build was stopped to free the slot (lead, 2026-10-22 00:45) and
is queued for a quiet window. The max RSS difference runs the other way
from the CPU time; with one run each it is not taken as a result.

## Decision 0338's per-loop marks instead (2026-10-22)

The same build with the loops with Float arithmetic marked
`llvm.loop.unroll.full` (-pragma-unroll-threshold=300, at threshold 150
otherwise; a clang wrapper rewriting the IR, the prototype of 0338),
paired in one bulk slot with the -O2 before 0333 (the flag stripped):

| build | user CPU | wall | max RSS | __text |
| --- | ---: | ---: | ---: | ---: |
| -O2, 0338's marks | 2,075 s | 2,713 s | 8.3 G | 2,395,012 |
| -O2 (before 0333) | 2,336 s | 3,927 s | 6.6 G | 2,395,012 |

xo's text is the same size with the marks (17 of its 8,801 loops have
Float arithmetic; none unrolls into more code). The two -O2 controls
(1,924 s here before, 2,336 s now) show the machine's noise: the marks
cost nothing measurable. nbody with 0338 (the gate's xo): 1,159.7 M
instructions, 0333's speed; collatz, fib, interp, sieve, sort and
strings within 0.1% of 0333, outputs identical.
