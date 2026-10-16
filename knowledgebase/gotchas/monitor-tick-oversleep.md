# The scheduler's monitor oversleeps under load

Status: Observed (2026-10-16, Mac, arm64; xo-4).

## Symptom

internal/llvm TestDeferredWakeBound (e53d33a3: a task deferred behind a
waker that computes 30 ms must run within 10 ms) failed on the Mac when
several sessions' builds ran: worst delays 12 to 43 ms, and some rounds
whose task had not run by the end of the 30 ms. The same test passed
with 1.3 to 2.2 ms at load 13.

## Cause

Deferred wakes (decision 0077's addendum) rely on the monitor thread's
1 ms timed futex wait (pool.xo futex_wait_us: __ulock_wait with a
timeout on macOS). With the CPUs oversubscribed (load 18 to 30 on 12
cores) the kernel ran the monitor 10 to 40 ms late: 19 to 45 of its
ticks per run took more than 3 ms (counted with debug counters).
pthread_set_qos_class_self_np(QOS_CLASS_USER_INTERACTIVE) on the
monitor did not change it. A direct futex wake of a worker, what main
did before for every hand off, is not subject to a timer.

## Fix

The test separates a working monitor from none: the waker computes 200
ms and the bound is 100 ms (worst 17.8 ms at load 26). A deferral's
worst case under load is the monitor's lateness, so a deferral whose
added latency matters (a Mutex's grant) needs a guard before it defers
(decision 0077's addendum).
