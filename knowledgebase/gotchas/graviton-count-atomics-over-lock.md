# On Graviton a shared count costs more than the lock around it

Status: Observed (2026-11-09, Xo Lang 4; decision 0574 stage 2, the
linux/arm64 timing host).

## Symptom

A C library's read through an XoHandle took 21 ns on host 1 (C: 1 ns).
Removing the table's lock (a lock free read with deferred frees) changed
nothing: 20.8 ns.

## Cause

The read counted and dropped three shared objects (the Any, its list of
one, the value): six LSE atomic operations at about 3 ns each on
Graviton. The lock's two were a third of it, and the fence a lock free
read needs costs about as much. perf showed the time in the wrapper's
and clibh.get's own instructions (the inlined atomics), not in a lock
wait.

## Fix / how to avoid

Count less, not lock less: read in place as a borrowed value (rt.obj_peek,
a borrowed root to ssa rc.xo) and count only what the callee keeps; the
read went to about 5 ns. Before removing a lock for speed on arm64,
count the atomic operations on the path, and profile with perf on host 1.
