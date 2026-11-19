# A pass that removes markers, turned off alone, sends them to the backends

Status: Observed (2026-11-19, task #358's pass-fuzz guard)

Symptom: `XO_PASSES=-addr_keep_drop` made every native backend fault in
its call lowering: `index -1 out of range for List ... at
arm64/state.xo:746` (arm64 and amd64, G.kind) and at llvm/gen.xo:725
(LLVM), on bench/perf/json, bench/perf/brotli and the closures golden
(amd64).

Cause: rc.xo's keep_addr_values (decision 0460) inserts `rt.addr_keep`
marker calls of type -1, and drop_addr_keeps removes them once the
releases are placed. With only the second pass off, the markers reached
the backends, which lowered them as runtime calls and looked up the
kind of type -1. TestGoldenPassesOff turns every optional pass off at
once (addr_keep too, so no markers exist) and each targeted pass alone,
and no golden targeted addr_keep_drop, so neither run saw it.

Fix: each backend (arm64 and x86-64 calls.xo, llvm call.xo, wasm
call.xo) emits nothing for `ssa.ADDR_KEEP`; the golden pass_addr_keep
targets addr_keep_drop and addr_keep. A pass that cleans up its own
markers gets a golden in passHuntTargets that turns the cleanup off
alone.
