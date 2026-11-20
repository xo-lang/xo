# Code added to an unwinder function made the fault rows slower: its size moved the inliner's choices

Status: Observed (2026-11-20, tasks #508 and #511)

## Symptom

A faster scan of the call site table written into `unwind.landing`
(std/native/llrt/unwind.xo) made bench/perf/fault slower on the native
arm64 backend: defer 942M to 1,013M instructions in the default build,
isolate 313M to 407M in a whole program build, owned 576M to 641M
(callgrind, linux/arm64 host 2). The LLVM build of the same source got
faster (defer 924M to 705M). The first version (a one byte fast path
with uleb as the fallback) lost the same way.

## Cause

`landing` is inlined into raise's loop over the frames. Longer, it and
its neighbours were inlined differently by the arm64 inliner, whose
choice depends on the sizes of the functions around a call; the scan
itself was not the cost. The whole program build's slower defer row
(1,594M against 940M by default) has the same root: with more inlined
into `deferred`, its call site table is longer, and the scan walks it
once per frame of a fault.

## Fix

The scan is a function of its own, `landing_uleb`, called once per
frame: `landing` keeps its size. With it defer is 844M by default and
1,184M whole program, owned 549M, isolate 311M. test/codegen/landing
checks that the scan calls nothing; scripts/lead/cgcheck-set.txt has
fault_defer and fault_owned rows. A change to unwind.xo's raise, step,
landing or lookup_frame is measured on all three fault rows in both
builds (default and whole program) before it is kept.
