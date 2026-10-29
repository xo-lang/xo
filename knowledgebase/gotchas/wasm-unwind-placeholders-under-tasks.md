# One waiting task made every wasm unwind exit build its placeholder results

Status: Observed (2026-10-29)

**Symptom.** compiler/wasm built for wasm (the playground's compiler) stayed near 70 MB after the closure-call suspension fix; 49% of its instructions were runs of thousands of loads and stores. `--annotate`'s per-function move counts (`(moves: op:way N, ...)`) showed every one of the 652 ssa.Builder methods with `struct:set 1952, ret:store 1045`.

**Cause.** The SSA builder ends a function in an unwind exit that hands a fault on (rt.fault_continue) and returns placeholder results: zeros, empty lists, "", and structs, tuples and enum variants of them; for a `var b: Builder` method that is a whole Builder, stored back into its home. compiler/wasm lowered such an exit as a zero return (unwind_block) only when no task of the program waits, since with waiting tasks a cancellation (not a fault) continues the caller with those results. ssa's fault_scans and cleanup_funcs took tasks, so every exit built and stored its placeholder leaf by leaf.

**Fix.** func.xo static_unwind: with waiting tasks, an unwind exit whose results are such constants (const_words) copies each wide result from one data segment of its leaves (static_words, one per distinct content) into its home or sret block with memory.copy; the values are the same, so a cancelled caller reads what it read before. Code 67.7 to 21.5 MB.

**Avoid.** Measure where a module's bytes go by what the backend emits per op (`--annotate` moves), not by guessing at copy sources: three guesses (edge moves, homes, nested updates) each moved the size by under 3%.
