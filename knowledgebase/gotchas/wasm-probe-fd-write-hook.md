# A wasm probe's fd_write hook took a real file's writes

Status: Observed (2026-11-12, Xo Lang 2, the /play/ REPL memory trace)

## Symptom

A scratch build of the playground's compiler that reported each big block request through
`fd_write` on fds 98 and 99 (the page's shim captured the JS stack there and returned 0) hung on
the third entry of a Mutex and Chan session: "the toolchain took longer than 120 s". The same
session passed on main and with the native compiler, so it looked like a bug in the change under
test.

## Cause

The browser's WASI shim numbers files as they are opened, and by that entry the file the compiler
writes `entry.wasm` to was fd 98 or 99. The hook swallowed its writes without setting the written
count, and `fs_write` retried forever. Pausing the worker over the DevTools protocol
(`Target.setAutoAttach` with `flatten`, then `Debugger.pause` in the worker's session) showed the
stack inside the hook.

## Fix

Give a probe's hook fds no shim hands out (1999998, 1999999), or its own import. To name a wasm
stack's `$fN` frames, build the same module with `XO_KEEP_WAT=1` (the WAT's `;; NAME` comment
before each `(func $fN`); a build with the same xo and sources is byte identical.
