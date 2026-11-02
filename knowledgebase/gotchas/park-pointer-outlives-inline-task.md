# A saved park can outlive the task, when main's fiber runs a task inline

Status: Observed (2026-11-01, Xo Lang 4, tier 35)

**Symptom.** `TestGoldenTasks/conc_reader_yield` failed now and then on Linux
under load: TSan reported `tasks.run_claimed`'s memset of a new context against
`pool.klock` or `pool.lock_slow`, in either order. It passed every idle run.
Built with XO_HEAP_GUARD=1, 366 of 500 loaded native runs crashed in lock_slow's
swap on an unmapped block.

**Cause.** A park belongs to a context, and a context on main's fiber may be an
inline task's rather than main's, so it is freed when that task ends. `park_wait`
published the park in `F_WAIT_I` for `count_main`, which read it and then locked
it with nothing holding the context. Main could resume, finish the task and free
the context in between. Taking and dropping the park's lock at task end
(1f5d91391) only waits for a thread already inside.

**Fix.** `count_main` claims the word with a CAS and restores it; main's fiber
clears it with a CAS before taking the park's lock, so it waits out a claim.
Before keeping a park's address outside a wait list or a lock that the waiter
must take, ask what keeps the context alive. A check for one suspect can stay
silent: a late `signal()` was ruled out first, and a mark in the dead context
showed the real reader in the next TSan report. To test a use after free in the
runtime without TSan, build with XO_HEAP_GUARD=1 and load the CPUs: freed records
are unmapped, so the bad access faults (TestHeapGuardMainInlinePark fails on its
third run without the fix).
