# A wasip1 program's stdin read failed with EAGAIN under node

Status: Observed (2026-11-03, golden stdin_long_line in TestSelfhostWasm's
leak check run, on the second linux/arm64 host during a full wasm gate)

Symptom: a wasip1/wasm program reading a large piped standard input
under node (compiler/wasm/run.mjs, node:wasi) stopped with
`error: input/output error` at a `read_line`, and the runner's call
count showed `fd_read ... (errno 6 x1)`. It passed when run alone; it
failed only when the machine was loaded.

Cause: node's WASI does not block on a pipe: when the pipe's writer (the
test's Go process feeding 220 KB of stdin.txt) has not refilled it yet,
`fd_read` on fd 0 answers EAGAIN (errno 6) instead of waiting, as a
nonblocking read does. compiler/wasm rt7.xo's `$wasi_stdin_read` turned
every nonzero errno into the read error (-5). A user piping a large file
into a wasm program under node hits the same failure; wasmtime's stdin
blocks, so it never showed there.

Fix: `$wasi_stdin_read` retries on EAGAIN after a wait on a relative
monotonic clock subscription (`poll_oneoff`, 1 ms doubling to 16 ms), so
the error never reaches the program. The module imports `poll_oneoff`
for a stdin reader without tasks too (the task runtime imports it
already). A clock wait rather than an fd_read subscription: it needs
nothing of the host but timers, which every WASI host has.

Tasks: this is wasip1's read (rt7_p1_text), which a program with tasks
also uses, and it waits for the whole instance, as the blocking fd_read
it stands in for always did: wasip1 has no per-descriptor wait that the
task runtime parks on, so other tasks do not run during either. A
wasip2 command reads stdin through wasi:cli/stdin's stream
(blocking-read, rt7.xo), which never answers EAGAIN and also waits for
the whole instance; parking only the reading task there (subscribe to
the stream's pollable, as rt9.xo's socket reads do) is a separate
change.

How to avoid: a wasm runtime call on a host file descriptor treats
EAGAIN (and EINTR) as "not yet", never as the error, since the WASI host
decides whether the descriptor blocks.
