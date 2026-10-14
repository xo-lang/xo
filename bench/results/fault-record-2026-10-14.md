# The fault record and fault exit in Xo (B06 part 1), 2026-10-14

Work item B06 of `bench/results/zero-c-audit-refresh-2026-10-13.md`,
except the Windows frame walk and its personality stub. Those wait for
the Windows runner. Branch `b06-faults`.

## What moved

- `std/native/llrt/fault.xo` keeps a context's fault record. That is
  xo_ctx's `faulting` and its message, location, and detail strings,
  read and written at offsets that xo_rt.c asserts (`fault.xo C_*`).
  - `set`, `detail`, `clear`, and `take` (task.isolate's catch).
  - `exit`: the report of a fault nothing caught. It flushes standard
    output, writes "fault: MSG at LOC" and the detail line in one write
    to standard error, and exits with status 70 through process.xo's new
    `os_exit`.
- The C calls them through cabi.xo's `fault_record_*` exports.
  - fault_exit, fault_set, fault_clear, xo_fault_detail, xo_fault_take,
    and xo_fault_raise's recording are one line each now.
  - dup_bytes is deleted: xo_fault_rethrow records a task's fault into
    the joining context through the same exports.
- The strings stay zero terminated in malloc'd blocks, because the C
  scheduler hands them from a faulted task to its scope and frees them
  with free. `rt.heap_large` returns the address after an 8 byte prefix,
  so fault.xo stores the block's own address
  (gotchas/heap-large-prefix-not-malloc-address.md).
- Unchanged:
  - the entry points of generated code (xo_fault, xo_fault_overflow, ...,
    whose messages cabi.xo already writes);
  - fault_raise (the unwind entry);
  - xo_fault_msg and xo_fault_loc (they build the string objects
    task.isolate returns);
  - the Windows walk.
- xo_rt.c: 57 lines deleted, 23 added. fault.xo is 120 lines with
  comments.

## Fault paths, Mac

bench/perf/fault, `/usr/bin/time -l` instructions retired, minimum of 5
runs. The same xo binary builds both programs: one with main's std
(df3e43b3), one with the branch's.

| workload | backend | main | B06 | ratio |
|---|---|---|---|---|
| none | arm64 | 16,041,866 | 16,054,451 | 1.001 |
| none | llvm | 13,746,056 | 13,648,880 | 0.993 |
| isolate | arm64 | 274,109,838 | 274,364,417 | 1.001 |
| isolate | llvm | 312,399,523 | 311,897,921 | 0.998 |
| defer | arm64 | 1,568,376,415 | 1,572,237,530 | 1.002 |
| defer | llvm | 1,306,183,269 | 1,308,220,980 | 1.002 |
| owned | arm64 | 911,466,368 | 917,172,937 | 1.006 |
| owned | llvm | 765,061,630 | 767,243,381 | 1.003 |

## Shared object frames between a fault and its catch

TestRuntimeFaultShared (internal/llvm, std/llrt/testdata/fault_shared.c
and .ll) raises a fault in a qsort comparator. The frames of the C
library's qsort then sit between the fault and a catcher whose landing
pad is under xo_x_personality. The test passes on the Mac. With
unwind.xo's dladdr lookup disabled it fails, so it covers the path that
no fault reached before. On Linux arm64 it covers the link_map lookup
(object_hdr).

## Behavior

On the Mac, with an xo built from the branch:
- internal/llvm's 157 goldens pass on arm64 and LLVM;
- TestRuntimeFaultResume, TestRuntimeTasks, TestRuntimeThreads,
  TestGoldenTasks (TSan), and TestRuntimeFaultShared pass.
