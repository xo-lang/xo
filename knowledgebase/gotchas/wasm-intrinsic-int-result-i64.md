# A wasm rt intrinsic declared `-> Int` must return i64

Status: Observed (2026-11-15, decision 0644 B1's `rt.wasi_sock_shutdown`)

## Symptom

A wasip2 golden that reaches the intrinsic (tls_close_write) fails
TestWasmCommandGoldens with `wasm-tools validate: error: func N failed
to validate`; wasmtime prints the reason, `type mismatch: expected i64,
found i32`. The preview 1 goldens (TestSelfhostWasm) pass, since the
golden is wasip1-unsupported and no other golden calls the intrinsic.

## Cause

An Xo `Int` is an i64 on wasm. compiler/wasm/rt9.xo's
`$wasi_sock_shutdown` declared `(result i32)`, the type of the
`$sk_unit` call it ends with, while std/native/rt.xo declares the
intrinsic `-> Int`; the caller's code expects an i64 on the stack.

## Fix

Declare the WAT function `(result i64)`, with `i64.const` for early
returns and `i64.extend_i32_u` after an i32 helper, as `$wasi_sock_err`
and `$wasi_sock_status` do. Without wasm-tools on the Mac, build the
golden with `xo build --targets=wasip2/wasm -o c.wasm .` and run
`wasmtime run -S inherit-network c.wasm`: a module that does not
validate fails to compile with the reason and the function's offset.
