# browser_wasi_shim logs every WASI call unless debug is false

Status: Observed (2026-10-10)

Symptom: the playground's console filled with `wasi: src/xo.mod` lines, one
per file system call.

Cause: `new WASI(args, env, fds)` calls `debug.enable(options.debug)`, and
`enable(undefined)` turns logging on.

Fix: pass `{ debug: false }` as the fourth argument (toolchain.js,
runner.js).
