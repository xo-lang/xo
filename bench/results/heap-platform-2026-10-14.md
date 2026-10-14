# The heap's platform calls in Xo (work item B21, part 1)

Date: 2026-10-14. Work item B21 of
`bench/results/zero-c-audit-refresh-2026-10-13.md` (decision 0180's
bar: within 5 percent of main's instructions on arm64 and LLVM).

## What moved

- std/native/llrt/heapos.xo (new): the memory of alloc.xo's 4 MiB chunks
  and of its large blocks (above mid_max, 256 KiB) is mapped from the
  system (mmap on Linux and macOS, VirtualAlloc on Windows) instead of
  the C library's malloc, realloc, and free; the central lists' lock is
  a lock word (pool.xo's lock_slow and futex_wake) instead of a pthread
  mutex. alloc.xo still calls them as rt.heap_* (its no allocation
  rule); the C's xo_heap_large, xo_heap_large_resize, xo_heap_release,
  xo_heap_grow, xo_heap_lock, and xo_heap_unlock are one line forwarders
  to cabi.xo's exports (deleted once the emitters name the exports).
- A large mapping is a size word, the block's prefix (minus its size),
  and the bytes, rounded up to 64 KiB. A large block given back goes to
  a cache of mappings (16, 64 MiB at most) that later large blocks reuse
  (one of at least the size and at most four times it); a resize within
  the mapping stays in place, and a growing block gets a mapping with
  room for twice its size. Without the cache bench/perf json parse took
  1.091 (arm64) and 1.106 (LLVM) times main's instructions: every big
  list was mapped, faulted in, and unmapped again, where malloc reused
  its large frees (gotchas/large-blocks-need-a-cache.md).
- The chunks mapped are a static word (rt.xo's table, words 15 to 19 are
  heapos.xo's), read through cabi.xo by the heap statistics and
  std/llrt/testdata tasks.c and alloc_threads.c.
- A sanitizer build (XO_TSAN, XO_ASAN) maps through the C library (the
  C runtime's xo_sched_rt services 11 and 12, which pool.xo's fiber
  stacks use for the same reason: ThreadSanitizer's mmap interceptor);
  C main tells heapos.xo through obj.heap_init.
- The fault strings (fault.xo dup) keep their prefix: the C's task_free
  gives them back through xo_heap_release instead of free.
- C: xo_rt.c loses malloc, realloc, and free of the heap, heap_chunk,
  the central lists' pthread mutex, and the chunk counter, and gains the
  forwarders and their comments: 55 lines deleted, 45 added; the net
  deletion comes with part 2 and when the forwarders go.
- Not yet: heap_slow (the heap records of threads: calloc, the list of
  all heaps and of spare ones, heaps_mu), part 2.

## macOS figures (darwin/arm64)

As bench/results/string-list-objects-2026-10-14.md's stage 3: A is main
4cde70db (its xo, built by seed 0.11.0, and its std), B the branch (its
xo, built by the same seed, and its std); min of 5 instructions retired,
net of the none row, two rounds, every checksum equal.

| Workload | Backend | A round 1 | B round 1 | B / A | A round 2 | B round 2 | B / A |
|---|---|---|---|---|---|---|---|
| text split | arm64 | 96,385,973 | 97,546,936 | 1.016 | 96,344,531 | 97,523,569 | 1.017 |
| text lines | arm64 | 153,650,885 | 155,758,447 | 1.016 | 153,656,878 | 155,766,749 | 1.017 |
| text int_fmt | arm64 | 79,966,537 | 79,956,194 | 1.000 | 79,974,387 | 79,855,203 | 0.999 |
| text quote | arm64 | 130,570,343 | 130,755,244 | 1.002 | 130,600,419 | 130,808,144 | 1.003 |
| text replace | arm64 | 82,629,430 | 82,795,799 | 1.003 | 82,628,357 | 82,817,385 | 1.004 |
| text debug | arm64 | 227,064,494 | 227,158,180 | 1.001 | 227,077,362 | 227,161,877 | 1.001 |
| text float_fmt | arm64 | 147,456,721 | 147,443,391 | 1.000 | 147,491,029 | 147,319,012 | 0.999 |
| text trim | arm64 | 154,651,607 | 156,237,531 | 1.012 | 154,541,395 | 156,218,013 | 1.013 |
| text split | llvm | 79,257,370 | 81,025,383 | 1.029 | 79,284,064 | 81,047,526 | 1.030 |
| text lines | llvm | 129,881,093 | 133,176,159 | 1.029 | 129,913,757 | 133,165,768 | 1.030 |
| text int_fmt | llvm | 64,868,794 | 64,932,396 | 1.000 | 64,897,751 | 64,961,469 | 1.002 |
| text quote | llvm | 112,701,361 | 112,058,791 | 0.992 | 112,688,595 | 112,050,426 | 0.993 |
| text replace | llvm | 65,450,714 | 65,782,017 | 1.006 | 65,449,770 | 65,770,712 | 1.007 |
| text debug | llvm | 201,370,411 | 201,090,872 | 0.998 | 201,362,924 | 201,064,411 | 0.998 |
| text float_fmt | llvm | 108,718,363 | 108,702,365 | 0.999 | 108,753,090 | 108,805,103 | 1.001 |
| text trim | llvm | 143,438,638 | 145,951,465 | 1.020 | 143,471,849 | 145,913,809 | 1.020 |
| json small | arm64 | 344,975,803 | 345,348,422 | 1.001 | 344,878,260 | 345,202,668 | 1.001 |
| json ints | arm64 | 234,429,355 | 234,866,844 | 1.001 | 234,320,564 | 234,799,842 | 1.002 |
| json strings | arm64 | 361,006,692 | 361,722,932 | 1.002 | 360,865,370 | 361,615,254 | 1.002 |
| json maps | arm64 | 337,709,175 | 338,446,654 | 1.002 | 337,705,250 | 338,419,817 | 1.002 |
| json parse | arm64 | 285,365,045 | 288,417,601 | 1.012 | 285,372,526 | 288,423,142 | 1.012 |
| json enc_small | arm64 | 150,869,939 | 150,918,264 | 0.999 | 150,838,066 | 150,919,931 | 1.001 |
| json enc_strings | arm64 | 191,898,138 | 191,924,019 | 0.999 | 191,877,343 | 191,922,391 | 1.000 |
| json small | llvm | 304,920,075 | 303,836,450 | 0.996 | 304,914,669 | 303,860,618 | 0.996 |
| json ints | llvm | 210,803,078 | 209,042,209 | 0.990 | 210,769,968 | 209,114,848 | 0.991 |
| json strings | llvm | 304,349,675 | 301,994,914 | 0.992 | 304,328,394 | 301,943,706 | 0.991 |
| json maps | llvm | 307,231,827 | 303,930,154 | 0.988 | 307,135,789 | 303,785,895 | 0.988 |
| json parse | llvm | 251,161,472 | 254,083,363 | 1.014 | 251,053,425 | 254,005,174 | 1.014 |
| json enc_small | llvm | 133,834,178 | 133,774,014 | 1.000 | 133,837,771 | 133,761,818 | 1.000 |
| json enc_strings | llvm | 182,744,842 | 182,658,458 | 1.000 | 182,726,635 | 182,678,133 | 1.000 |
| alloc small | arm64 | 292,135,535 | 292,022,696 | 1.000 | 292,150,900 | 292,089,541 | 1.000 |
| alloc strings | arm64 | 301,390,686 | 303,149,611 | 1.006 | 301,302,826 | 303,048,058 | 1.006 |
| alloc maps | arm64 | 303,572,843 | 303,669,457 | 1.000 | 303,555,961 | 303,669,261 | 1.000 |
| alloc mixed | arm64 | 472,509,141 | 472,537,596 | 1.000 | 472,546,406 | 472,525,038 | 1.000 |
| alloc grow | arm64 | 67,122,844 | 67,107,910 | 1.000 | 67,097,946 | 67,102,231 | 1.000 |
| alloc tasks | arm64 | 878,018,839 | 885,641,982 | 1.009 | 880,623,246 | 887,896,579 | 1.008 |
| alloc small | llvm | 247,893,608 | 247,916,103 | 1.000 | 247,944,174 | 247,911,709 | 1.000 |
| alloc strings | llvm | 285,031,992 | 277,825,598 | 0.973 | 285,045,300 | 277,842,880 | 0.974 |
| alloc maps | llvm | 267,123,010 | 266,627,865 | 0.998 | 267,113,264 | 266,656,382 | 0.998 |
| alloc mixed | llvm | 462,287,369 | 462,276,148 | 1.000 | 462,334,933 | 462,356,615 | 1.000 |
| alloc grow | llvm | 64,461,562 | 64,446,119 | 1.000 | 64,491,125 | 64,420,023 | 0.999 |
| alloc tasks | llvm | 778,711,593 | 784,604,692 | 1.008 | 779,593,270 | 784,619,845 | 1.007 |
| nested push | arm64 | 17,894,934 | 18,015,512 | 1.007 | 18,010,079 | 17,969,779 | 0.998 |
| nested strappend | arm64 | 18,902,673 | 18,936,368 | 1.002 | 18,939,834 | 18,937,573 | 1.000 |
| nested walk | arm64 | 31,687,368 | 31,718,900 | 1.001 | 31,711,430 | 31,718,280 | 1.000 |
| nested push | llvm | 15,072,425 | 15,111,381 | 1.003 | 15,072,367 | 15,108,297 | 1.002 |
| nested strappend | llvm | 15,942,320 | 15,978,973 | 1.002 | 15,928,474 | 16,000,865 | 1.005 |
| nested walk | llvm | 27,687,937 | 27,670,819 | 0.999 | 27,670,436 | 27,730,804 | 1.002 |

Every row is within the bar: 0.973 to 1.030. text split and lines are
1.016 and 1.017 (arm64) and 1.029 and 1.030 (LLVM), trim 1.013 and
1.020; not page faults or chunks (one chunk, 400 page reclaims, in A
and B alike) and not a large block's growth (room for twice the size
changed nothing); next: count the large block calls of text split, and
time the cache's lock and search against malloc's free path.

The self build (each xo first rebuilt by itself, then building main's
selfhost/xo with main's std): A 106,742,882,093 and 106,356,112,789, B
107,287,297,465 and 107,675,450,558 instructions (1.005 and 1.012); the
two outputs byte identical.

Behavior on the Mac: internal/arm64 TestGoldenRun and internal/llvm
TestGolden (every golden, with the leak check), TestRuntime* (with
tasks.c and alloc_threads.c's chunk checks), TestGoldenTasks,
TestLeakCheck*.
