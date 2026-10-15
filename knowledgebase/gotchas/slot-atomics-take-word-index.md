# rt.slot_* take a word index, not a byte offset

Status: Observed (2026-10-14, B20: poolsetup.xo, a hang of every
concurrent program, the compiler included)

Symptom: after the pool's setup moved to Xo, every program that spawned
a task hung at the first scope's close. No worker thread started, and
the root thread waited in pool.xo park_wait for tasks that never ran. A
compiler built with that runtime hung too, because xo itself runs tasks,
so the goldens' builds timed out one by one.

Cause: `rt.slot_store(p, P_NCPU, n, RELEASE)` with P_NCPU = 8, the byte
offset of the pool record's ncpu. The slot atomics' second argument is a
word index: the cell is the word at `xs + 8 * i` (std/native/rt.xo). So
the store went to byte 64, the inject queue's lock (P_IMU), which then
looked held, while ncpu stayed 0. pool.xo itself uses both kinds of
constant (P_NCPU = 8 for mem_load, P_ALIVE = 4 for slot_load), which
makes the mixup easy.

Fix: pass the field's address and index 0, `rt.slot_load[Int](p +
P_NCPU, 0, ACQUIRE)`, as budget.xo and tasks.xo do; keep byte offsets
and word indexes in differently named constants (pool.xo's *_I names).
