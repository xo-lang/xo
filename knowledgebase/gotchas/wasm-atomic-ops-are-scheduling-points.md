# wasm: Atomic's operations are scheduling points, so they cannot be inline loads

Status: Anticipated (2026-10-13, decision 0200)

Symptom (avoided): lowering `Atomic.get` and friends inline in the
shared SSA (decision 0200's `rt.slot_load` on the value's cell) would
make a wasm program that spins on an Atomic (`while !flag.get() {}`)
never let another task run, and would change the interleavings of
seeded tests (core 7.4 lists Atomic operations among the scheduling
points); the cell is also at byte 8 of rt3.xo's object, not byte 128.

Cause: the wasm runtime runs one thread with a cooperative scheduler;
rt3.xo's `$atomic_get` and friends start with `call $yield_point`. The
SSA builders are backend independent.

Fix: selfhost/wasm `atomics_to_runtime` rewrites, before emission,
`rt.slot_*` on a handle (an Atomic) back to `rt.atomic_get`, `set`,
`swap`, `cas`, and calls of the native prelude's `llrt_atomic_add` and
`llrt_atomic_max` to `rt.atomic_add` and `rt.atomic_max`: wasm behaves
exactly as before the port. Only `rt.slot_*` on a `List[Int]` lowers
inline there (plain loads and stores, one thread).
