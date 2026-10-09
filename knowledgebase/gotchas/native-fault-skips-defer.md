# A fault on the native backends does not run deferred code

Status: Observed (2026-10-08, decision 0070)

Symptom: a program whose block holds a `defer` and then faults prints the
deferred output on the Go backend (`runs during fault`, then the outer
defers) but not with `--backend=llvm` or `--backend=arm64`, where the
fault message is the last thing printed.

Cause: the Go backend runs deferred code while the fault's panic unwinds
the goroutine. The native backends exit the process from the faulting
operation; only errors, early exits, and cancellation (explicit unwind
checks, decision 0068) run deferred code there.

Fix: none needed for correct programs (a fault is a bug). Golden tests
shared by every backend must not fault with deferred code pending:
`internal/llvm/testdata/golden/defer_blocks` is internal/gen's
`defer_blocks` without its last block. Spec core 3.5 notes the
difference.
