# Native stdout is buffered: output must be flushed before a wait

Status: Anticipated (2026-10-08, decision 0069)

Symptom (expected without the fix): a native program prints `ready` and
waits for a signal or a connection, and nothing shows on a pipe until it
exits; a test that waits for the line before it sends SIGTERM hangs. The
Go backend writes each print at once.

Cause: `std/llrt/xo_rt.c` buffers stdout (64 KiB) and flushed it only at
exit, before a fault, and before stderr output.

Fix: prints set an atomic `out_dirty` flag, and every wait of a context
(`park_wait_n`: channels, locks, awaits, joins, timers, sockets) flushes
the buffer first. Output of a program that never waits still appears at
exit. `internal/llvm/signal_test.go` depends on it.
