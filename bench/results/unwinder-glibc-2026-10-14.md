# The fault unwinder in Xo on glibc Linux and macOS (B05 stage 3), 2026-10-14

Decision 0221 stage 3 (zero-C audit item B05,
`bench/results/zero-c-audit-refresh-2026-10-13.md`): a fault on glibc
linux/arm64 unwinds through std/native/llrt/unwind.xo instead of
libgcc's `_Unwind_ForcedUnwind`, xo_rt.c's fault_stop, and the C
personality. On macOS libunwind stays and only the personality routine
is Xo. Branch `b05-stage3`.

## Workload

`bench/perf/fault` (main.xo, measure.sh): 20,000 faults per workload,
each caught by `task.isolate`.

- isolate: raised 8 frames deep, with no deferred code.
- defer: through 9 frames that each have a defer, so a landing pad and a
  raise again in every frame.
- owned: through 5 frames that each own a list, which the landing pads
  release.
- none: the setup alone.

The figures are the minimum of 5 runs. The baseline is the same program
built with main's std (the C path): on the Mac with the same xo binary
and origin/main's std (4999c6ef), on Linux CI's measure suite (main's xo
and std against the branch's).

## macOS (darwin/arm64, the bar of decision 0180)

`/usr/bin/time -l` instructions retired, head 41af5dcf against main.
Only the personality routine changed here (libunwind still walks), so
these are within noise of 1.

| workload | backend | C | Xo | ratio |
|---|---|---|---|---|
| none | arm64 | 16,589,071 | 16,603,419 | 1.001 |
| none | llvm | 14,120,623 | 14,140,418 | 1.001 |
| isolate | arm64 | 2,738,344,805 | 2,740,793,683 | 1.001 |
| isolate | llvm | 3,727,775,130 | 3,747,682,585 | 1.005 |
| defer | arm64 | 13,415,925,137 | 13,473,765,362 | 1.004 |
| defer | llvm | 13,801,514,315 | 13,898,088,205 | 1.007 |
| owned | arm64 | 7,419,354,739 | 7,450,169,881 | 1.004 |
| owned | llvm | 7,894,013,813 | 7,966,705,792 | 1.009 |

## Linux (linux/arm64, CI measure suite, r8g big runner)

perf stat `instructions:u`. Run 38618253973, at the head 41af5dcf
(the rule set by register bits, the CIE cache, the program's table
found once per thread):

| workload | backend | C (libgcc) | Xo | ratio |
|---|---|---|---|---|
| none | arm64 | 232,714 | 176,368 | 0.758 |
| none | llvm | 231,063 | 174,879 | 0.757 |
| isolate | arm64 | 780,300,225 | 444,239,431 | 0.569 |
| isolate | llvm | 821,078,348 | 332,337,946 | 0.405 |
| defer | arm64 | 3,330,000,222 | 2,041,339,447 | 0.613 |
| defer | llvm | 2,923,678,339 | 1,653,617,935 | 0.566 |
| owned | arm64 | 1,876,940,277 | 1,172,119,498 | 0.624 |
| owned | llvm | 1,962,238,358 | 1,018,037,954 | 0.519 |

At 96e2387d (run 38618051277, before the table cache) the ratios were
0.410 to 0.642.

The first version (run 38617655625, commit b026136d) was 1.30 to 1.49
times the C's on the arm64 backend (isolate 1.487, defer 1.300, owned
1.296) and 0.945 to 1.003 on LLVM. Each step went over all 96 register
rules, each frame cleared and copied the 1,568 byte rule set, and each
frame parsed its CIE and ran the CIE's instructions again. In 96e2387d
the rule set keeps a bit per register with a rule (two words), so
reset, copy, and step touch only those registers. step updates the
register set in place, and the last CIE's fields and initial rules are
cached in the thread's record. Why "none" (no fault) is 0.76 is not
investigated; it is about 56,000 instructions of start up.

## Behavior

- Mac: internal/llvm's 157 goldens pass on arm64 and LLVM (`xo dev
  goldens --force`), as do TestRuntimeFaultResume, TestRuntimeTasks, and
  TestRuntimeThreads.
- Linux CI auto (merged with main), run 38616952357 at b026136d: short,
  the full tests of internal/selfhost, internal/arm64, internal/llvm,
  internal/gen, and cmd/xo, and the fixed point all pass; the same at
  the head 41af5dcf, run 38618248328.

## Not done

- linux/amd64 (stage 2): glibc linux/amd64 keeps libgcc's forced unwind
  (XO_UNWIND_LIBGCC in xo_rt.c), so the C fault_stop, forced unwind,
  personality, and LSDA reader are still in the source. There is no
  amd64 CI runner yet to verify the x86_64 trampolines on.
- macOS pays about 137,000 instructions per fault in libunwind (isolate
  arm64: 2.74e9 / 20,000), against about 22,000 for the Xo walker on
  Linux. An Xo walker of `__unwind_info` (compact unwind) would cut that.
  It would also retire the C stop function, which must otherwise end the
  program itself (gotchas/libunwind-stop-must-not-return.md).
