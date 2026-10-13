# ThreadSanitizer on Linux runs out of address space for fiber stacks

Status: Observed (2026-10-12, EC2 c8g, Amazon Linux 2023, kernel 6.18,
clang 15.0.7), fixed in std/llrt/xo_rt.c (fibers_init); observed again
with 8 MiB stacks (2026-10-13, batch 7 full run on 40b08de1, r8g.8xlarge),
fixed with 2 MiB stacks.

Symptom: on linux/arm64, the ThreadSanitizer builds of
TestGoldenThreads/conc_stress, TestGoldenThreads/clock_timers,
TestGoldenTasks/conc_stress, and TestRuntimeEventLoop exit 70 with
`fault: out of memory`. (TestH2Sanitizers/thread, "connection refused",
also passed once this and the memcpy fix landed; its cause was not traced
separately.) Near the limit the program can instead exit 66 or 133 or
hang: ThreadSanitizer's own mappings fail too ("failed to allocate
noreserve ... StackStore", then a CHECK failure that can hang).
The same tests pass on the Mac, and the same programs pass on Linux
without the sanitizer. With 8 MiB stacks it showed only under load (the
full suite with every package in parallel), never alone.

Cause: mmap of a fiber stack (MAP_NORESERVE) fails with ENOMEM: address
space, not memory. ThreadSanitizer on Linux reserves nearly all of the
48 bit address space as PROT_NONE for its shadow and meta memory and
leaves the program's mmaps 31.8 GiB on arm64 (a C loop under
`-fsanitize=thread` mapped 508 stacks of 64 MiB, or 1019 of 32 MiB, then
ENOMEM; 2026-10-13, clang 15.0.7), shared with ThreadSanitizer's own
mappings and the threads' stacks. A fiber stack is mapped for every task
parked at once (plus small caches), so the need is the program's peak
of parked tasks times the stack size:

- 64 MiB stacks ran out after about 880 fibers (2026-10-12).
- 8 MiB stacks ran out at 2988 to 3150 fibers (23.3 to 24.6 GiB; the
  new `xo: fiber stack mmap failed ... with N stacks of 8 MiB mapped`
  line, 390 failures, 2026-10-13).
- TestGoldenThreads runs conc_stress four times at once in one process
  (-DXO_RT_TEST_THREADS=4); each copy parks up to 2000 senders on a full
  channel. Its fiber peak (XO_RTSTATS=1, `fiber_peak`) grows with load,
  because a loaded receiver drains the channel more slowly. On
  r8g.8xlarge (32 CPUs): 724 to 1209 alone; 16, 32, and 64 copies of the
  binary at once reached 2867, 5378, and 5296. On the 12 core Mac: 2398
  to 3141 alone, up to 4942 with 8 at once. 5378 fibers of 8 MiB is 42
  GiB, over the 31.8.

Measured on r8g.8xlarge, 2026-10-13 (probe binaries built as
TestGoldenThreads builds conc_stress; two rounds of N copies at once):

| build | 16 at once | 32 at once | 64 at once |
|---|---|---|---|
| 40b08de1, default (8 MiB) | 3 of 31 failed | 35 of 60 | 114 of 126 |
| 40b08de1, XO_LLRT_C=rc (8 MiB) | 2 of 32 | 48 of 63 | 114 of 124 |
| fix with XO_TASK_STACK_MB=8 | 2 of 32 | 41 of 60 | 110 of 127 |
| fix (2 MiB) | 0 of 32 | 0 of 64 | 0 of 128 |

It is not a leak: every run that finished reported `xo leakcheck: 0 live
of 266112 allocations` (four copies) or `0 live of 66528` (one), the same
counts on 40b08de1 and on main after the Xo allocator (7b83dd53), with
the reference counts in Xo (default) and in C (XO_LLRT_C=rc), on the Mac
and on Linux. Peak RSS of the four copies under ThreadSanitizer stayed
270 to 407 MB on Linux (333 to 637 MB on the Mac), alone and under load.

Fix: under ThreadSanitizer on Linux (XO_TSAN and __linux__) the default
task stack is 2 MiB; XO_TASK_STACK_MB still overrides. The memory need is
explicit in TestGoldenThreads (fiberBound): conc_stress may have at most
4 x 2000 + 9 x CPUs + 64 fibers at once (8352 on 32 CPUs, 16.3 GiB of 2
MiB stacks); the test reads `fiber_peak` and fails above the bound,
which would be a fiber leak. With the fix, TestGoldenThreads,
TestGoldenTasks, TestRuntimeTasks, and TestRuntimeEventLoop pass on
r8g.8xlarge. Programs that need deep recursion are tested without the
sanitizer (deepRecursion in internal/llvm/threads_test.go).

How to find it next time: a failing fiber stack mmap now prints `xo:
fiber stack mmap failed (<errno>) with N stacks of M MiB mapped` before
the fault; N x M near 24 GiB under ThreadSanitizer on Linux is this
limit. `XO_RTSTATS=1` prints `fiber_live` and `fiber_peak`. Other
xo_oom call sites still print only `fault: out of memory`.

Related: a kernel with 39 bit addresses (512 GiB) would hit the same
limit without any sanitizer at about 8000 parked tasks of 64 MiB.
