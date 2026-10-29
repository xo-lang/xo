# A struct of more than 1000 leaves makes a wasm module no engine loads

Status: Observed (2026-10-29)

**Symptom.** The playground compiler (compiler/wasm built for wasm) of
0.34.0 built, but Node refused it: `WebAssembly.Module(): param count of
1038 exceeds internal limit of 1000`. The same for xo built for
wasip1/wasm (Xo Lang 3).

**Cause.** V8 accepts at most 1000 parameters and 1000 results in a
function type. The wasm backend passes a value as its leaves, and
ssa.Builder grew past 1000 (1039; 1045 in a tuple). Three places took or
returned it whole: the drop helpers (rc_addr_fn reloaded a wide value's
leaves from memory and called the by-value helper; drop_fields_fn took
every leaf), and ssa.build_program_* and the drivers' build steps, which
return a Builder.

**Fix.** compiler/wasm: a wide struct or tuple (more than WIDE_RC leaves)
is retained and released field by field from memory (home.xo mem_rc,
rc_addr_fn), never through a helper taking all its leaves; a function
returning more than MAX_RESULTS (1000) leaves, called only directly, takes
one more parameter, a zeroed block the caller allocates before the call's
resume point, stores its results there, and the caller reads and frees it
(call.xo sret_block, also in home_call). The backend fails the build
(check_arity) when any function still has more than 1000 parameters or
results, instead of writing a module that does not load.

**Avoid.** A wasm module the backend wrote is not proof that it loads: run
it (`node compiler/wasm/run.mjs`), as the gate of a compiler/wasm change.
