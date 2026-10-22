# An unused load of a bad address is not a trap

Status: Observed (2026-10-22, Xo Lang 2)

Symptom: a checkpoint written as `let _x = rt.mem_load(0x5ace0, 0, 8)`
behind a failed check never faulted, so a heap corruption hunt under
Wine read "every check passed" for three runs.

Cause: rt.mem_load has no side effect the optimizer must keep; its
unused result is dead and the load is removed.

Fix: trap with a store, `rt.mem_store(0x5ace0 + 16 * k, 0, 1, 8)` (a
distinct address per checkpoint; Wine's +seh trace prints it as
info[1]). Check a trap by forcing it once before trusting silence.
