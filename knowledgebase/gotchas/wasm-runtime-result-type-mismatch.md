# A wasm runtime function whose result type differs from the SSA's only fails when a golden first reaches it

Status: Observed (2026-10-13, decision 0193 step 4)

## Symptom

TestSelfhostWasm: codex_gaps fails with `wasm trap: WebAssembly.Module():
Compiling function #211:"f21" failed: local.set[0] expected type i64,
found call of type i32`. The module does not even instantiate; the
function named is the program's, not the runtime's, and the offending
call is not the one the .wat text makes obvious.

## Cause

The WAT runtime's `$clock_sleep_or_cancel` (compiler/wasm rt3.xo)
returned `i32`, while the SSA builder types the call `rt.clock_sleep_or_cancel`
as an Int (`i64`), as the C runtime's `int64_t xo_clock_sleep_or_cancel`
returns. Nothing checks the WAT runtime's signatures against the SSA's
types; V8's validation finds it only when a module calls the function.
codex_gaps was "unsupported" on wasm until std/json's parser became Xo
(it decodes JSON), so the mismatch had never been compiled.

## Fix

`$clock_sleep_or_cancel` returns `i64` (`i64.extend_i32_u` of
`$clock_wait`). To find such a call: the trap's `@+offset` is a byte
offset in the module; decode the call there (opcode 0x10, then the
function index as LEB128) against the type and name sections. A small
script that does it (no wabt on the machine) took a minute to write.
When a golden starts building on wasm for the first time, expect runtime
paths it reaches that no golden reached before.
