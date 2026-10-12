# Node 22 crashes running allocation heavy wasm with the concurrent GC

Status: Observed (2026-10-12), Node 22.22.1 on darwin/arm64.

**Symptom.** A module of the WebAssembly backend (decision 0179) that
allocates much (the golden `rc_recursive`: a chain of a million boxes)
exits with SIGSEGV (status 139) in about one run in three, at a
different point each time, with no trap and nothing on stderr. lldb
shows `v8::internal::InnerPointerToCodeCache::GetCacheEntry` (the stack
walk of a garbage collection). The same module run ten times with
`--single-threaded-gc` never crashes; `--no-liftoff`, `--liftoff-only`,
`--no-wasm-tier-up`, a larger or smaller worker stack, and dropping the
name section do not help. Small modules that only grow memory or return
many values did not reproduce it.

**Cause.** A race in V8's concurrent garbage collector while wasm frames
are on the stack (memory.grow makes the external memory grow, which
starts collections); not in the module, whose memory accesses are
checked.

**Fix.** `selfhost/wasm/run.mjs` calls
`v8.setFlagsFromString('--single-threaded-gc')` before it starts the
worker. Run modules through run.mjs (or pass the flag); retest without
it when Node is upgraded, and check the playground's browsers, which run
V8 too.
