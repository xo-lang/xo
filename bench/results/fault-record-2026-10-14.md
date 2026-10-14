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

## Part 2: the entry points (decision 0228)

Branch b06-entries. Generated code calls cabi.xo's `xo_x_fault_raise_*`
exports, and the C `xo_fault_*` forward to them for older xo's code.
bench/perf/fault on the Mac, `/usr/bin/time -l` instructions retired,
minimum of 5 runs. Main cd150967 is built by its own xo, the branch by
its own xo (its emitters call the exports).

| workload | backend | main | part 2 | ratio |
|---|---|---|---|---|
| none | arm64 | 16,123,140 | 15,987,082 | 0.992 |
| none | llvm | 13,736,071 | 13,566,554 | 0.988 |
| isolate | arm64 | 275,737,453 | 285,772,886 | 1.036 |
| isolate | llvm | 313,286,770 | 301,525,266 | 0.962 |
| defer | arm64 | 1,573,594,085 | 1,578,643,932 | 1.003 |
| defer | llvm | 1,308,342,586 | 1,312,384,685 | 1.003 |
| owned | arm64 | 917,079,389 | 925,447,742 | 1.009 |
| owned | llvm | 765,991,898 | 764,190,243 | 0.998 |

isolate on arm64 costs about 500 instructions more per fault. That is
the export's frame and the message's copy into the thread's buffer
(unwind.xo msg_buf), which the C wrote straight into its stack buffer.
It is within decision 0180's 5 percent.

On the Mac, with the branch's xo:
- internal/llvm's 157 goldens pass on arm64 and LLVM;
- TestRuntimeFault* (FaultResume, FaultShared), TestRuntimeTasks,
  TestRuntimeThreads, and TestGoldenTasks (TSan) pass;
- the arm64 asm goldens change only in the 10 fault calls.

## Part 3: the Windows walk

Branch b06-windows. Before this branch, xo_rt.c's Windows fault_raise
walked the frames in C. unwind.xo `win_raise` now does it in Xo:
- RtlCaptureContext, then for each frame RtlLookupFunctionEntry and
  RtlVirtualUnwind with UNW_FLAG_UHANDLER, through sys_windows.xo
  declarations. CONTEXT offsets are given for x64 and arm64.
- The call site table goes through the LSDA reader shared with the other
  OSes (`landing`).
- At a landing pad, RtlRestoreContext with the exception in Rax (X0).
Windows now uses the common fault_raise. The C Windows walk and
lsda_landing are deleted: xo_rt.c is 116 lines deleted and 24 added
against main. The C xo_personality stub stays, since it is the handler
the walk looks for.

Windows (CI's xo-windows runner, c7i.4xlarge, windows/amd64, LLVM),
bench/perf/fault/measure-windows.ps1 through the script suite.
Measure-Command wall clock milliseconds, minimum of 5 runs; the same
xo.exe builds against main's std (830c6cb4) and the branch's. Windows
has no instruction count at hand, so this is a best effort comparison,
not decision 0180's bar.

| workload | C walk | Xo walk (run 38637550599) | Xo walk with the lookup cache (run 38638886262) |
|---|---|---|---|
| none | 14.2 | 13.5 | 14.4 |
| isolate | 35.5 | 38.6 | 33.3 (0.94) |
| defer | 119.2 | 146.6 | 130.6 (1.10) |
| owned | 71.4 | 85.3 | 73.4 (1.03) |

The C column is from run 38638886262; run 38637550599's base was 13.7,
36.2, 119.8, and 71.3.

The first Xo walk was 1.07 to 1.22 times the C's. Every raise walks the
unwinder's own frames too (the cabi export, raise, win_raise), each a
RtlLookupFunctionEntry and a RtlVirtualUnwind. A cache of the function
entry lookup by pc (256 entries in the scratch record) brought it to
0.94 to 1.10. defer is the slowest at 1.10: each of its 9 raises per
fault walks those frames again.

CI windows: windows/amd64 157 of 157 goldens, windows/arm64 157 built,
the LLVM runtime tests ok (run 38633766335, before the cache; run
38640009496 at the head). CI auto green (runs 38633770035 and
38640013605).
