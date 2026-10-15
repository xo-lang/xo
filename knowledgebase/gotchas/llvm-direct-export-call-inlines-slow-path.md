# Calling an Xo export directly from LLVM IR lets its slow path inline

Status: Observed (2026-10-15, the generated-code entries' batch E1)

Symptom: after the LLVM emitter called heapos.xo's exports
(xo_x_heap_big, xo_x_heap_grow, xo_x_heap_lock and the rest) directly
instead of the C forwarders xo_heap_big and the rest, bench/perf/alloc
took 1.10 times the instructions on `tasks` and 1.03 on `strings`
(arm64 unchanged). Marking the calls alwaysinline or nounwind changed
nothing.

Cause: the C forwarders were declared in the program's IR and defined in
the separately compiled C runtime, so each call was an inlining barrier.
The exports are defined in the same LLVM module as alloc.xo's fast
paths, so LLVM inlined the heap's slow paths (a new chunk, a big block,
the central lists' lock) into them, which grew the hot functions and
their register pressure. Found by bisecting the batch into four
variants (bench/perf/alloc tasks and strings, LLVM, 5 runs each).

Fix: the emitter calls those exports with a call site attribute group
`{ nounwind noinline }` (selfhost/llvm gen.xo `#8`, call.xo rt_call),
keeping the barrier the C forwarder gave: tasks 0.995 and strings 0.951
of the C path. Moving a hot path's callee from a C forwarder to a
direct call needs the same check: decide per call whether it should
inline (obj.xo's list and string entries do, `#6`) or stay out of line.
