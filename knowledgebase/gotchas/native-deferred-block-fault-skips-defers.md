# A fault inside a deferred block skips the remaining defers (native backends)

Status: Observed (Xo Lang 2, 2026-10-17, while writing decision 0258).

## Symptom

`defer { let _ = boom("x") }`, where `boom` faults, behaves differently
on the native backends (LLVM and arm64) from the Go backend. On the
native backends the defers registered before it do not run, and a
program with tasks can end in `fault: XO-F001: deadlock: every task is
blocked`. The Go backend runs every remaining defer, as core 5.6
requires. `defer boom("x")` behaves the same on every backend.

Seen by rewriting internal/llvm/testdata/golden/fault_defer_chain's
seven `defer boom(...)` lines as `defer { let _ = boom(...) }`.
TestGolden, TestGoldenRun, TestGoldenSplit and TestGoldenTasks then
failed on fault_defer_chain. For example, case 2 lost "first defer runs
(1)", case 8 lost its sibling's output, and case 9 deadlocked.

## Cause

Not found yet. A deferred block is lowered differently from a deferred
call. The fault's unwinding apparently leaves the defer chain at the
block, not at the next registered defer.

## Fix

Open. Decision 0258 exempts `defer` statements from XO0209, so the
golden keeps `defer boom(...)` and nothing requires the block form.
To reproduce, apply the rewrite above to the golden and run
`go test ./internal/llvm -run TestGolden/fault_defer_chain`.
