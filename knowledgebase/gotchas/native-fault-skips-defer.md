# A fault on the native backends does not run deferred code

Status: Resolved (2026-10-09, decision 0085; observed 2026-10-08,
decision 0070)

Symptom: a program whose block holds a `defer` and then faults printed
the deferred output on the Go backend (`runs during fault`, then the outer
defers) but not with `--backend=llvm` or `--backend=arm64`, where the
fault message was the last thing printed. A fault in a task exited the
process (no 500 from an HTTP handler, no `task.isolate`), leaving held
Mutexes and leaked references behind.

Cause: the Go backend runs deferred code while the fault's panic unwinds
the goroutine. The native backends exited the process from the faulting
operation; only errors, early exits, and cancellation (explicit unwind
checks, decision 0068) ran deferred code there.

Fix: decision 0085. A fault is a forced unwind of the platform unwinder
through landing pads that reuse the SSA unwind paths
(`internal/ssa/landing.go`): deferred code runs, scopes close, Mutexes
unlock, references are released, a task's fault fails its scope, main
exits with status 70, `task.isolate` and HTTP handlers catch it. Goldens
`internal/llvm/testdata/golden/fault_*` and `http_fault` compare all
three backends (leak check 0 where the program goes on). The trimmed
`defer_blocks` golden of internal/llvm can keep its shape; internal/gen's
full `defer_blocks` now prints the same on all three backends.
