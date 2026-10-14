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
- The chunks mapped are a static word (rt.xo's table: words 15 to 19 are
  heapos.xo's; 14 is fault.xo's since decision 0228), read through
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

## macOS figures (darwin/arm64)

As bench/results/string-list-objects-2026-10-14.md's stage 3: A is main
010ed599 (its xo, built by seed 0.11.0, and its std), B the branch (its
xo, built by the same seed, and its std); min of 5 instructions retired,
net of the none row, two rounds, every checksum equal.

| Workload | Backend | A round 1 | B round 1 | B / A | A round 2 | B round 2 | B / A |
|---|---|---|---|---|---|---|---|
| text split | arm64 | 96,434,316 | 97,542,050 | 1.016 | 96,341,021 | 97,534,072 | 1.017 |
| text lines | arm64 | 153,706,619 | 155,824,174 | 1.016 | 153,644,963 | 155,793,766 | 1.017 |
| text int_fmt | arm64 | 80,017,911 | 80,004,633 | 1.000 | 79,995,971 | 79,975,006 | 1.000 |
| text quote | arm64 | 130,617,669 | 130,837,119 | 1.002 | 130,564,550 | 130,819,498 | 1.002 |
| text replace | arm64 | 82,648,078 | 82,837,315 | 1.004 | 82,555,000 | 82,751,558 | 1.004 |
| text debug | arm64 | 227,159,556 | 227,229,610 | 1.000 | 227,086,594 | 227,202,912 | 1.001 |
| text float_fmt | arm64 | 147,503,737 | 147,463,776 | 1.000 | 147,490,714 | 147,505,466 | 1.000 |
| text trim | arm64 | 154,698,087 | 156,318,760 | 1.013 | 154,666,695 | 156,253,357 | 1.012 |
| text split | llvm | 79,281,112 | 81,094,805 | 1.029 | 79,254,023 | 81,042,936 | 1.028 |
| text lines | llvm | 129,925,343 | 133,226,805 | 1.029 | 129,895,828 | 133,194,252 | 1.029 |
| text int_fmt | llvm | 64,878,564 | 64,975,762 | 1.000 | 64,869,034 | 64,957,934 | 1.000 |
| text quote | llvm | 112,660,107 | 112,020,212 | 0.992 | 112,697,209 | 112,060,935 | 0.992 |
| text replace | llvm | 65,445,067 | 65,815,554 | 1.006 | 65,418,375 | 65,823,681 | 1.007 |
| text debug | llvm | 201,391,801 | 201,133,316 | 0.998 | 201,351,009 | 201,072,221 | 0.998 |
| text float_fmt | llvm | 108,730,022 | 108,847,330 | 1.000 | 108,649,599 | 108,813,111 | 1.001 |
| text trim | llvm | 143,447,351 | 145,944,761 | 1.020 | 143,438,162 | 145,931,080 | 1.019 |
| json small | arm64 | 344,883,463 | 345,264,261 | 1.001 | 344,863,589 | 345,209,624 | 1.001 |
| json ints | arm64 | 234,320,145 | 234,769,446 | 1.002 | 234,282,057 | 234,800,952 | 1.002 |
| json strings | arm64 | 360,850,545 | 361,610,198 | 1.002 | 360,871,150 | 361,635,896 | 1.002 |
| json maps | arm64 | 337,693,511 | 338,427,912 | 1.002 | 337,696,147 | 338,429,923 | 1.002 |
| json parse | arm64 | 285,353,427 | 288,446,508 | 1.012 | 285,331,481 | 288,428,637 | 1.012 |
| json enc_small | arm64 | 150,841,212 | 150,904,110 | 1.000 | 150,788,159 | 150,925,989 | 1.000 |
| json enc_strings | arm64 | 191,872,585 | 191,943,391 | 1.000 | 191,802,337 | 191,915,129 | 1.000 |
| json small | llvm | 304,909,908 | 303,823,682 | 0.996 | 304,908,369 | 303,825,419 | 0.996 |
| json ints | llvm | 210,768,361 | 209,157,544 | 0.992 | 210,805,231 | 209,031,387 | 0.990 |
| json strings | llvm | 304,319,398 | 301,973,815 | 0.992 | 304,341,446 | 301,827,160 | 0.991 |
| json maps | llvm | 307,142,984 | 303,821,143 | 0.988 | 307,102,170 | 303,824,308 | 0.988 |
| json parse | llvm | 251,052,783 | 254,010,363 | 1.014 | 251,082,730 | 254,006,572 | 1.014 |
| json enc_small | llvm | 133,820,942 | 133,749,504 | 1.001 | 133,821,884 | 133,768,185 | 1.000 |
| json enc_strings | llvm | 182,749,685 | 182,584,841 | 1.000 | 182,738,213 | 182,674,840 | 1.000 |
| alloc small | arm64 | 292,155,276 | 292,149,636 | 1.000 | 292,146,556 | 292,030,111 | 1.000 |
| alloc strings | arm64 | 301,319,375 | 303,163,165 | 1.006 | 301,373,906 | 303,137,628 | 1.006 |
| alloc maps | arm64 | 303,530,564 | 303,661,466 | 1.000 | 303,566,529 | 303,673,435 | 1.000 |
| alloc mixed | arm64 | 472,553,975 | 472,517,720 | 1.000 | 472,570,432 | 472,512,896 | 1.000 |
| alloc grow | arm64 | 67,126,349 | 67,099,852 | 0.998 | 67,141,162 | 67,088,520 | 0.999 |
| alloc tasks | arm64 | 880,500,558 | 884,318,067 | 1.004 | 878,765,713 | 884,633,916 | 1.007 |
| alloc small | llvm | 247,839,010 | 247,836,016 | 1.000 | 247,892,593 | 247,893,565 | 1.000 |
| alloc strings | llvm | 285,018,839 | 277,862,418 | 0.974 | 285,020,101 | 277,815,599 | 0.973 |
| alloc maps | llvm | 267,113,476 | 266,674,237 | 0.998 | 267,112,324 | 266,636,917 | 0.998 |
| alloc mixed | llvm | 462,278,048 | 462,311,832 | 1.000 | 462,196,958 | 462,214,147 | 1.000 |
| alloc grow | llvm | 64,453,748 | 64,449,133 | 1.000 | 64,468,352 | 64,379,536 | 0.999 |
| alloc tasks | llvm | 779,636,222 | 784,827,199 | 1.007 | 778,875,181 | 784,062,479 | 1.007 |
| nested push | arm64 | 18,015,691 | 17,977,673 | 0.998 | 18,019,547 | 17,984,998 | 0.998 |
| nested strappend | arm64 | 18,920,118 | 18,807,112 | 0.994 | 18,930,555 | 18,904,050 | 0.999 |
| nested walk | arm64 | 31,675,358 | 31,692,480 | 1.001 | 31,692,722 | 31,695,141 | 1.000 |
| nested push | llvm | 15,052,829 | 15,187,695 | 1.009 | 15,033,008 | 15,177,600 | 1.010 |
| nested strappend | llvm | 15,939,192 | 16,000,008 | 1.004 | 15,935,796 | 16,058,021 | 1.008 |
| nested walk | llvm | 27,637,889 | 27,809,403 | 1.006 | 27,659,502 | 27,828,726 | 1.006 |

Every row is within the bar: 0.973 to 1.029. text split and lines are
1.016 and 1.017 (arm64) and 1.028 to 1.029 (LLVM), trim 1.012 and
1.020; not page faults or chunks (one chunk, 400 page reclaims, in A
and B alike) and not a large block's growth (room for twice the size
changed nothing); next: count the large block calls of text split, and
time the cache's lock and search against malloc's free path.

The self build (each xo first rebuilt by itself, then building main's
selfhost/xo with main's std): A 107,225,767,501 and 107,116,938,864, B
106,871,172,933 and 106,652,720,019 instructions (0.997 and 0.996); the
two outputs byte identical.

Behavior on the Mac: internal/arm64 TestGoldenRun and internal/llvm
TestGolden (every golden, with the leak check), TestRuntime* (with
tasks.c and alloc_threads.c's chunk checks), TestGoldenTasks,
TestLeakCheck*, and go test -short (cmd/xo TestAttenuationFakes with
it).
