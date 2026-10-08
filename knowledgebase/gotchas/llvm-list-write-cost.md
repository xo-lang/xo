# List writes in the LLVM backend needed inline fast paths, TBAA, and a uniqueness cache

Status: Observed (2026-10-08, `bench/perf/llvm`)

Symptom: the first LLVM backend build of the sieve ran 5x slower than Go
(0.50 s against 0.10 s), nbody 3x, the interpreter 1.8x, although clang
-O2 with LTO compiled everything into one function.

Cause: every element write called the runtime (`xo_list_mut(&xs, ...)`,
which copies a shared list) and every push `xo_list_push_slot(&xs, ...)`.
LTO did not inline them, and passing the binding's address kept the list
pointer in memory, so each iteration reloaded it, re-checked the shared
flag, and re-checked bounds; LLVM could not move header loads (length,
flag) past element stores because both are plain memory.

Fix (internal/llvm/fn.go): the fast paths are inline IR and only the
copy is a call (`xo_list_unshare`, `xo_list_grow`, marked cold, returning
the new list that the emitter stores back), so bindings stay in SSA
registers. Header and element accesses carry distinct TBAA tags
(`!3`, `!5` in `runtimeDecls`), so header loads hoist past element
stores. A per block cache (`fn.uniq`) remembers that a binding was just
made unshared, so the second to sixth writes in one nbody iteration skip
the check; it is dropped at block merges, user calls, retains, and
stores to the binding. Result: sieve 0.50 -> 0.13 s, nbody 0.49 ->
0.17 s, interp 0.24 -> 0.12 s.
