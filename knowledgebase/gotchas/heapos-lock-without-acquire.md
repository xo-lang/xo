# A lock with the wrong slot order constant hands a block out twice

Status: Observed (2026-10-14, work item B21)

Symptom: the first heapos.xo (the central lists' lock as a lock word)
passed every test on the Mac, but on linux/arm64 the fixed point's
stage s1 (xo building selfhost/xo with several emit jobs) segfaulted
after about 5 minutes, printing nothing; with XO_EMIT_JOBS=1 it passed.
Under gdb the crash was alloc.alloc popping a free list whose head was
1: a reference count written into a block that was still on a free
list, so a block had been handed out twice.

Cause: heapos.xo defined its own slot orders as ACQUIRE = 2 and RELEASE
= 3, but rt.xo's are 0 relaxed, 1 acquire, 2 release, 3 acq_rel, 4
seq_cst. The lock's compare and swap ran with release order, so on
arm64 the loads of the central lists inside the critical section could
be satisfied before the lock was taken, and two threads took the same
magazine. The Mac ran without failing (fewer threads, other timing).
Static word collisions with branches merged meanwhile (B17 took words
15 and 16, which heapos.xo used then) confounded the bisection.

Fix: the constants are rt.xo's (ACQUIRE 1, RELEASE 2), as pool.xo and
rc.xo have them. Copy the order constants from rt.xo's comment (or an
existing module) rather than from memory, and test a new lock on
linux/arm64 with many threads (the fixed point does that), not only on
the Mac.
