# A new std test module that reaches a socket fails the tier's stdwasm step

Status: Observed (tier 919, 2026-11-21)

## Symptom

The tier's `stdwasm` step (scripts/gate.sh, decision 0610) fails with
`MODULE: no summary` and, in the module's lines,
`xo: the wasm backend does not support the network (rt.net_status): WASI
preview1 has no sockets` (or `proc.run`, or a C call such as
`rt.c_xo_sqlite_..._sqlite3_libversion`). Tier 919 at the 0.55.0
candidate had 8 such modules: grpc, native/llrt/mysql, smtp, sqlite, sse,
testing, ui and websocket.

## Cause

The wasm backend rejects a whole test module's build when any function
it links needs sockets, processes or C, so one network test fails the
module, not one test. A module the backend cannot build must be listed
in compiler/xo/wasmskip.xo `wasm_module_skip` with its reason
(decision 0437: a skip names its reason, never silently absent). The
stdwasm step runs only in the tier, not per push, so 7 test modules
added by the Std Proven push (2026-11-15 to 2026-11-18, 78 to 124 test
modules) went unlisted; the step passed last at tier 77 (46f76126c).

## Fix

Add the module to `wasm_module_skip` (NO_SOCKETS, or the process or C
reason), then check it with
`XO_TEST_EXEC="node --no-warnings $PWD/compiler/wasm/run.mjs" bin/xo test --backend=wasm --count=1 --std-module=MODULE`:
the summary reads `ok: 0 passed, 0 failed, N skipped (N wasm: ...)`.
When adding a std test module, run that line once before pushing.
