# std/heap changes

## 2026-11-03: feature: std/heap, a priority queue with remove and fix

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 13: a scheduler cancels and reschedules timers); decision 0472 (Proposed)
- Go reference: container/heap (heap.go: Init, Push, Pop, Remove, Fix)
- Test: internal/stdtest/testdata/corpus/heap (container/heap's array after every operation of 150 random sequences, heap.Init of 120 random slices, the moved callback on a job queue, copies) on arm64, LLVM and the Go backend; bench/perf/heap twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`heap.new`, `tracked`, `from_list`, `push`, `pop`, `peek`, `get`, `remove`, `fix`, `len`, `is_empty`, `to_list` (spec/std.md 56). The comparison is a function, and `moved(x, i)` stands for Go's `Swap` bookkeeping.
