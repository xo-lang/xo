# wasm: a native-only rt call in a dead arm fails the whole build

Status: Observed (2026-10-15, TestSelfhostWasmFsWrite in the selfhost
merge tier, bisected to a4ed9cd0)

Symptom: compiler/wasm stops a program that writes files with "the
wasm backend does not support the call `rt.heap_release` (stage 2)",
although nothing calls it at run time in a WASI module.

Cause: a4ed9cd0 moved the file operation outcome into Xo:
`sysio.set_err` returns through `rt.fs_set_err` when `sysio.layer()` is
0 (wasm), and otherwise keeps the path and message in a block from
`rt.heap_large`/`rt.heap_release`. compiler/wasm emits every call of a
reachable function, dead arms included, and fails on an rt call it does
not know. The merge tier was the first run to see it; -short skips the
wasm builds.

Fix: compiler/wasm/call.xo's list of calls that fault when run (with
sys6, fs_set_err and the other native-only calls) also has heap_large,
heap_release and heap_large_resize. When std/native code under a
`layer() == 0` guard adds another native-only rt call, add it there
too, or the wasm tests fail at merge tier.
