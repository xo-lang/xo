# --race reported races on the runtime's own records (task, fiber, worker)

Status: Observed (2026-11-02)

Symptom: `xo build --race` on linux/arm64 reported "Atomic write ... in a
runtime function" races for task programs (conc_scopes, conc_mutex,
async_preempt_regs, conc_stress): sched.task_claim, fiber_yield,
pool.preempt, rq_put and others, each against a "Previous write" by
`malloc` or `memset` on another thread. Same at v0.35.0.

Cause (three separate causes, one class: TSan sees a record's creation as a
plain write by the creating thread, and the record is then handed to
another thread through words TSan cannot order):
1. linux `--race` is the LLVM backend (native arm64 has no TSan runtime for
   linux/arm64), and the runtime's atomics still reach TSan as
   `__tsan_atomic64_*` accesses, so the hand off of a task record to the
   worker that claims it (queue words, relaxed) had no edge.
2. The race heap's `malloc` (heapos.xo, libcmem.xo) records the block's
   reset as a write by the allocating thread; a fiber record used first by
   another thread raced with it.
3. start_worker's `memset` of the new worker's record, read by other threads.

Fix: `rq_put` releases the record (`sanhook.release(x & -2)`) and `run_item`
and the callers of `sched.task_claim` acquire it (task_claim itself cannot
call sanhook: XO1201, it would be able to fault); the race heap's `malloc`
and the worker record's `memset` run under `sanhook.ignore_writes`, so
TSan resets the shadow instead of recording the thread's write.

Not the fix (tried, negative): private `xo_aarch64_` outline atomic helpers
(the native backend's idea; the linux --race build is LLVM); a `noinline`
attribute twin for the runtime functions.

How to avoid: a new hand off of a runtime record between threads needs a
sanhook release/acquire pair or an ignored write at creation; check with
`TestRaceSyncTable` (its unsynchronized cases must still report) and the
conc_* goldens under `--race` on a linux host.
