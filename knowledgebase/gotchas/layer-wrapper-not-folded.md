# A wrapper around sysio.layer() keeps the native arms in a wasm build

Status: Observed (2026-10-17, wasm file handles)

Symptom: a program that opens a file failed to build for wasm with `the
wasm backend does not support the call rt.heap_big (stage 2), in
alloc.alloc`, although std/native/llrt/fsops.xo's open_file returns
through rt.fs_open (the WASI runtime, compiler/wasm rt4.xo) on layer 0
before any native code.

Cause: the SSA builders fold only a call of `sysio.layer()` itself (or
`netconn.os()`) compared with a literal (compiler/ssa expr.xo
layer_call). fsops.xo tested `let l = layer()` then `if l == 0`, through
its own `fn layer() -> Int { sysio.layer() }`: neither the wrapper nor
the local folds, so every native arm (raw_file's tasks.new_handle, the
native heap, the darwin and linux system calls) stayed in the module and
reached calls the wasm backend does not have.

Fix: guard the WASI arm with `if sysio.layer() == 0 { ... }` written out
(fsops.xo's 25 layer 0 arms since 2026-10-17), then `let l = layer()`
for the native arms. When a wasm build stops on an unexpected native
call, its message names the function that made it (compiler/wasm
call.xo), which leads to the guard that did not fold.
