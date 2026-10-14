# The heap's platform calls in Xo (work item B21, part 1)

Date: 2026-10-14. Work item B21 of
`bench/results/zero-c-audit-refresh-2026-10-13.md` (decision 0180's
bar: within 5 percent of main's instructions on arm64 and LLVM).

## What moved

- std/native/llrt/heapos.xo (new): the memory of alloc.xo's 4 MiB chunks
  and of its big blocks (above mid_max, 256 KiB) is mapped from the
  system (mmap on Linux and macOS, VirtualAlloc on Windows) instead of
  the C library's malloc, realloc, and free; the central lists' lock is
  a lock word (pool.xo's lock_slow and futex_wake) instead of a pthread
  mutex. alloc.xo calls them as rt.heap_big, rt.heap_big_resize,
  rt.heap_big_release, rt.heap_grow, rt.heap_lock, and rt.heap_unlock
  (its no allocation rule allows only rt.heap_* calls); the C's xo_heap_*
  of the same names are one line forwarders to cabi.xo's exports
  (deleted once the emitters name the exports).
- rt.heap_large, rt.heap_large_resize, and rt.heap_release stay the C
  library's malloc: they are no longer only alloc.xo's big blocks but the
  runtime's records (testworld.xo, pool.xo's worker and fiber records,
  fault.xo's strings, stdio.xo's buffer, obj.xo's drop worklist), and the
  C frees some of them with free (xo_testing.c, task_free). The first
  version mapped those too; cmd/xo TestAttenuationFakes crashed on
  xo_testing.c's free of a test world record
  (gotchas/heap-large-is-the-runtime-malloc.md). An allocator of the
  runtime's own for them is the next step (with B04's xo_nolibc.c).
- A big mapping is a size word, the block's prefix (minus its size),
  and the bytes, rounded up to 64 KiB. A big block given back goes to a
  cache of mappings (16, 64 MiB at most) that later big blocks reuse (one
  of at least the size and at most four times it); a resize within the
  mapping stays in place, and a growing block gets a mapping with room
  for twice its size. Without the cache bench/perf json parse took
  1.091 (arm64) and 1.106 (LLVM) times main's instructions: every big
  list was mapped, faulted in, and unmapped again, where malloc reused
  its large frees (gotchas/large-blocks-need-a-cache.md).
- The chunks mapped are a static word (rt.xo's table: words 22 to 26 are
  heapos.xo's; 14 to 16 and 18 to 21 went to fault.xo, ctx.xo, and
  tzhost.xo meanwhile), read through
  cabi.xo by the heap statistics and std/llrt/testdata tasks.c and
  alloc_threads.c.
- A sanitizer build (XO_TSAN, XO_ASAN) maps through the C library (the
  C runtime's xo_sched_rt services 11 and 12, which pool.xo's fiber
  stacks use for the same reason: ThreadSanitizer's mmap interceptor);
  C main tells heapos.xo through obj.heap_init.
- C: xo_rt.c loses heap_chunk, the central lists' pthread mutex, and the
  chunk counter, and gains the forwarders and their comments: 31 lines
  deleted, 46 added (15 more); the net deletion comes with part 2 (heap_slow: the heap records of threads,
  calloc, the lists of all and of spare heaps, heaps_mu) and when the
  forwarders go.

## A race found by the fixed point (linux/arm64)

The first CI run failed only the fixed point: on linux/arm64 the
branch's xo segfaulted about 5 minutes into stage s1 (building
selfhost/xo; 176 s and no crash with XO_EMIT_JOBS=1, against 21 to 38 s
for main's), printing nothing; the Mac passed everything. gdb (run
38645392364) stopped in alloc.alloc popping a free list whose head was
1: a block handed out twice. A temporary switch sending each part back
to the C (runs 38645272128 to 38649163658) was confounded by B17 taking
static words 15 and 16, which heapos.xo used then. The cause: heapos.xo
defined its own slot orders as ACQUIRE 2 and RELEASE 3, where rt.xo's
are 1 acquire and 2 release (0 relaxed, 3 acq_rel, 4 seq_cst), so the
central lists' lock took the word with a release compare and swap, no
acquire, and threads read the lists before holding it. With rt.xo's
values (6544efe3) s1 passes in 34 and 20 s (run 38650558123); strace -c
of it: 901 futex calls, 5,483 mmap, 1,870 munmap, 44,239 brk. The 5x
time was the same race (threads contending a lock that did not exclude).

## macOS figures (darwin/arm64)

As bench/results/string-list-objects-2026-10-14.md's stage 3: A is main
0d25c89c (its xo, built by seed 0.11.0, and its std), B the branch (its
xo, built by the same seed, and its std); min of 5 instructions retired,
net of the none row, two rounds, every checksum equal.

| Workload | Backend | A round 1 | B round 1 | B / A | A round 2 | B round 2 | B / A |
|---|---|---|---|---|---|---|---|
| text split | arm64 | 96,207,325 | 97,354,707 | 1.017 | 96,198,983 | 97,383,455 | 1.017 |
| text lines | arm64 | 153,521,612 | 155,611,461 | 1.016 | 153,506,312 | 155,622,666 | 1.017 |
| text int_fmt | arm64 | 79,843,465 | 79,849,749 | 1.001 | 79,865,058 | 79,857,309 | 1.001 |
| text quote | arm64 | 130,426,133 | 130,627,359 | 1.002 | 130,435,623 | 130,622,109 | 1.002 |
| text replace | arm64 | 82,458,889 | 82,668,077 | 1.004 | 82,460,388 | 82,670,428 | 1.004 |
| text debug | arm64 | 226,918,338 | 226,969,635 | 1.001 | 226,906,750 | 227,039,574 | 1.001 |
| text float_fmt | arm64 | 147,309,925 | 147,321,146 | 1.001 | 147,310,888 | 147,285,934 | 1.000 |
| text trim | arm64 | 154,511,799 | 156,104,842 | 1.013 | 154,505,428 | 156,096,487 | 1.013 |
| text split | llvm | 79,341,871 | 81,151,169 | 1.030 | 79,398,871 | 81,147,476 | 1.028 |
| text lines | llvm | 130,022,439 | 133,163,987 | 1.028 | 130,021,504 | 133,309,549 | 1.029 |
| text int_fmt | llvm | 64,988,528 | 65,065,117 | 1.001 | 64,997,103 | 65,104,995 | 1.001 |
| text quote | llvm | 112,802,509 | 112,165,646 | 0.993 | 112,784,712 | 112,148,813 | 0.992 |
| text replace | llvm | 65,511,278 | 65,917,172 | 1.008 | 65,572,325 | 65,859,559 | 1.005 |
| text debug | llvm | 201,454,857 | 201,199,647 | 0.998 | 201,436,567 | 201,199,404 | 0.998 |
| text float_fmt | llvm | 108,826,730 | 108,932,877 | 1.001 | 108,846,713 | 108,942,423 | 1.000 |
| text trim | llvm | 143,563,337 | 146,066,761 | 1.020 | 143,582,398 | 146,012,523 | 1.019 |
| json small | arm64 | 344,789,496 | 345,145,863 | 1.001 | 344,823,453 | 345,197,577 | 1.001 |
| json ints | arm64 | 234,235,346 | 234,710,631 | 1.002 | 234,225,787 | 234,663,884 | 1.002 |
| json strings | arm64 | 360,785,351 | 361,545,729 | 1.002 | 360,806,277 | 361,513,280 | 1.002 |
| json maps | arm64 | 337,619,066 | 338,345,438 | 1.002 | 337,636,178 | 338,319,787 | 1.002 |
| json parse | arm64 | 285,249,572 | 288,415,651 | 1.013 | 285,179,200 | 288,280,684 | 1.012 |
| json enc_small | arm64 | 150,778,137 | 150,804,698 | 1.000 | 150,708,426 | 150,779,224 | 1.000 |
| json enc_strings | arm64 | 191,774,573 | 191,831,712 | 1.000 | 191,791,139 | 191,825,708 | 1.000 |
| json small | llvm | 304,993,344 | 303,848,148 | 0.996 | 304,985,817 | 303,871,892 | 0.996 |
| json ints | llvm | 210,817,645 | 209,077,437 | 0.990 | 210,797,245 | 209,182,700 | 0.991 |
| json strings | llvm | 304,392,311 | 301,954,030 | 0.991 | 304,379,445 | 301,965,577 | 0.991 |
| json maps | llvm | 307,196,326 | 303,888,429 | 0.988 | 307,146,730 | 303,897,369 | 0.988 |
| json parse | llvm | 251,122,361 | 253,977,581 | 1.013 | 251,068,346 | 254,059,753 | 1.014 |
| json enc_small | llvm | 133,889,188 | 133,826,414 | 1.000 | 133,898,631 | 133,836,710 | 1.000 |
| json enc_strings | llvm | 182,759,904 | 182,671,440 | 1.000 | 182,808,796 | 182,753,018 | 1.000 |
| alloc small | arm64 | 292,070,369 | 292,050,653 | 1.000 | 291,976,934 | 292,038,167 | 1.000 |
| alloc strings | arm64 | 301,251,254 | 303,052,739 | 1.006 | 301,273,662 | 303,090,754 | 1.006 |
| alloc maps | arm64 | 303,467,129 | 303,586,526 | 1.000 | 303,480,879 | 303,589,074 | 1.000 |
| alloc mixed | arm64 | 472,435,814 | 472,437,846 | 1.000 | 472,352,732 | 472,443,856 | 1.000 |
| alloc grow | arm64 | 67,027,288 | 67,035,380 | 1.000 | 67,008,707 | 67,022,410 | 1.000 |
| alloc tasks | arm64 | 880,844,649 | 885,009,061 | 1.005 | 880,077,454 | 885,456,416 | 1.006 |
| alloc small | llvm | 247,953,181 | 247,942,572 | 1.000 | 247,981,872 | 247,948,010 | 1.000 |
| alloc strings | llvm | 285,040,884 | 277,784,265 | 0.973 | 285,091,699 | 277,878,698 | 0.973 |
| alloc maps | llvm | 267,154,968 | 266,687,830 | 0.998 | 267,178,943 | 266,646,434 | 0.998 |
| alloc mixed | llvm | 462,309,515 | 462,346,009 | 1.000 | 462,373,721 | 462,363,296 | 1.000 |
| alloc grow | llvm | 64,499,716 | 64,486,108 | 1.000 | 64,466,546 | 64,496,826 | 1.001 |
| alloc tasks | llvm | 779,590,472 | 784,471,669 | 1.006 | 777,899,228 | 786,219,127 | 1.011 |
| nested push | arm64 | 17,953,447 | 17,933,590 | 0.999 | 17,898,679 | 17,915,733 | 1.001 |
| nested strappend | arm64 | 18,907,049 | 18,860,805 | 0.998 | 18,841,937 | 18,843,804 | 1.000 |
| nested walk | arm64 | 31,635,446 | 31,645,543 | 1.000 | 31,647,980 | 31,639,960 | 1.000 |
| nested push | llvm | 15,101,530 | 15,229,461 | 1.008 | 15,106,127 | 15,200,053 | 1.006 |
| nested strappend | llvm | 15,979,573 | 16,105,388 | 1.008 | 15,975,645 | 16,084,403 | 1.007 |
| nested walk | llvm | 27,676,716 | 27,851,687 | 1.006 | 27,607,055 | 27,849,954 | 1.009 |

Every row is within the bar: 0.973 to 1.030. text split and lines are
1.016 and 1.017 (arm64) and 1.028 to 1.030 (LLVM), trim 1.013 and
1.020; not page faults or chunks (one chunk, 400 page reclaims, in A
and B alike) and not a large block's growth (room for twice the size
changed nothing); next: count the large block calls of text split, and
time the cache's lock and search against malloc's free path.

The self build (each xo first rebuilt by itself, then building main's
selfhost/xo with main's std): A 106,666,658,218 and 106,474,308,496, B
106,830,673,971 and 106,894,122,761 instructions (1.002 and 1.004); the
two outputs byte identical.

Behavior on the Mac: internal/arm64 TestGoldenRun and internal/llvm
TestGolden (every golden, with the leak check), TestRuntime* (with
tasks.c and alloc_threads.c's chunk checks), TestGoldenTasks,
TestLeakCheck*, and go test -short (cmd/xo TestAttenuationFakes with
it).
