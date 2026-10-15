# A C `#if` on a macro defined further down the file is always false

Status: Observed (2026-10-15, found while inventorying xo_rt.c).

Symptom: sanitizer builds mapped the Xo heap with raw system calls,
out of sight of ThreadSanitizer's and AddressSanitizer's mmap
interceptors, although xo_rt.c had a switch for exactly that
(heap_libc) and heapos.xo a C library arm for it. Nothing failed:
TestGoldenASan and TestGoldenTasks passed, and the arm's runtime
services (rt.sched_rt ops 11 and 12) had been deleted from the C in
694cc542 without anything noticing.

Cause: `#if defined(XO_TSAN) || defined(XO_ASAN)` sat at line 220 of
xo_rt.c, but XO_TSAN and XO_ASAN are defined from `__has_feature` at
lines 1,834 to 1,850, and no build passes them with -D. The
preprocessor evaluates an `#if` where it stands, so heap_libc was always
0 and the C library arm was dead code.

Fix: the decision moved to run time in Xo, as for the fiber stacks
(decision 0232): heapos.xo map and unmap call sanhook.xo heap_map and
heap_unmap (sys_c.xo mmap and munmap) when sanhook.sanitized()
(rt.sym_present of the sanitizers' weak symbols), and count those
mappings in static word 26. TestSysCTSan checks that a ThreadSanitizer
build's heap went through the C library (the sysc.heap_libc op). In C,
define a feature macro before its first test, or test the compiler's
own (`__has_feature`, `__SANITIZE_THREAD__`) where it is used.
