# Two wasm "unsupported" lists, read by two tests

Status: Observed (2026-11-05, Xo Lang 1, full tier 52)

Symptom: full tier 52 (at ae8f0928d) failed `internal/selfhost`
TestWasmCommandGoldens/goldens on frame_large_value and rc_large_offsets,
though 61ba87d38 had listed both as unsupported on wasm the day before.
The Mac never showed it: the test skips without wasmtime and wasm-tools.

Cause: the goldens run on wasm in two tests with a list each.
TestSelfhostWasm (wasip1, Node) reads
`internal/selfhost/testdata/wasm_unsupported.txt`; TestWasmCommandGoldens
(wasip2 components, wasmtime) reads `wasm_cmd_unsupported.txt`. A golden
that cannot build for wasm at all (a backend limit, not a missing WASI
capability) fails both and belongs in both; a golden that needs sockets
or other preview 2 features is listed only in the first. Each test also
fails when a listed golden passes, so the second list must lose the line
in the same push that fixes the golden.

Fix: when a golden cannot build for the wasm backend, list it in both
files with the same reason; when a push makes it work, drop both lines.
Prove the second with `XO_WASM_GOLDENS=NAME go test ./internal/selfhost
-run '^TestWasmCommandGoldens$' -v` on a linux/arm64 offload host (it has
wasmtime and wasm-tools). Fixed forward in cc959f9f9.
