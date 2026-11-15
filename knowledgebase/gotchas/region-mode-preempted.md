# A preempted task left the heap region mode on

Status: Observed (2026-11-14, xo's self build on linux/arm64 host 3).

## Symptom

`XO_HEAP_REGION_STATS=1` printed "release 0.00 ms; pooled -1 units, ...
1 threads in region mode" in most self builds with decision 0637's
crawling workers on, and max RSS was 5.2 to 5.6 GB against 3.8 GB for a
serial build (`XO_FE_SPEC=1`), whose release ran every time.

## Cause

Region mode (decision 0633) belongs to the thread: region_on points the
thread's TLS_HEAP at its region record and counts the thread active. The
runtime preempts a fiber that has run 10 ms while other work waits
(decision 0286 step 3). With the crawling workers running there is always
work waiting, so the main builder's lazy check (check_lazy, in region
mode) was preempted and resumed on another thread. Its region_off ran
there, found that thread not in region mode, and did nothing: the first
thread stayed in region mode and the count stayed at 1, so the release
refused to run (region_rel_begin returns -1 while a thread is active).
0633 had assumed a front end task never yields inside a parse or a check;
preemption made that false.

## Fix

region_on and region_off call pool.held(+1/-1), as the runtime's spin
locks do, so no preemption poll or signal yields the task while it is in
region mode. A region window must also never block (a channel operation,
an await, a lock, I/O that parks): `XO_LOCKCHECK=1` stops the process
with "a fiber switched away holding a runtime lock" when one does, which
is how to check a new window. The crawling workers' lazy checks (decision
0637 stage 3) did switch fibers under XO_LOCKCHECK=1 (the main builder's
did not; which call switches is not pinned down), so a worker does not
enter region mode (compiler/ssa/contract.xo check_lazy). scripts/gate.sh's
selfbuild step runs with XO_LOCKCHECK=1 and XO_HEAP_REGION_STATS=1 and
fails when the release did not run with 0 threads in region mode.

A skipped region_off costs memory, not correctness: the count changes only
when a thread enters or leaves the mode, so it is the number of threads in
region mode, and the release runs only at 0; the stale thread's later
allocations go to region chunks (freed by address as any region block)
and stay out of the pool.
