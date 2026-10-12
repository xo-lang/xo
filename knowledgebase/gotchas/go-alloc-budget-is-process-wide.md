# A Go-backend allocation budget counts the whole process

Status: Anticipated (2026-10-11, decision 0173)

Symptom: `task.with_budget(clock, task.budget(alloc: N), f)` returns
`Err(Exceeded("alloc"))` although f itself allocated well under N, on a
busy server where other tasks allocate at the same time.

Cause: Go keeps no allocation count per goroutine. The Go backend reads
the process wide `/gc/heap/allocs:bytes` (runtime/metrics) when the
budget starts and at each charge, so every allocation in the process
during the call counts against the budget. It is an upper bound: the
budget fails closed, never open.

Fix: size `alloc` ceilings for the process, or bound the work with
`steps` (deterministic, per budget) or `cpu` (running time of the
budget's own threads) instead. The native backends are planned to count
per task in std/llrt's allocator, where the count is exact.
