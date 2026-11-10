# std/heap changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus heap on go, arm64 and llvm

std/heap.xo now holds the bodies of std/native/heap.xo (removed), and the Go
backend compiles them from it as its own package, in place of the hand written std/rt/heap.go (removed): the Go
backend now runs the same heap code as the native backends. Heap's fields are private (they were declared with `_`
names for that). The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: feature: std/heap, a priority queue with remove and fix

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-30 backend survey (std-coverage.md rank 13: a scheduler cancels and reschedules timers); decision 0472 (Accepted, owner 2026-11-03)
- Go reference: container/heap (heap.go: Init, Push, Pop, Remove, Fix)
- Test: internal/stdtest/testdata/corpus/heap (container/heap's array after every operation of 150 random sequences, heap.Init of 120 random slices, the moved callback on a job queue, copies) on arm64, LLVM and the Go backend; bench/perf/heap twincheck
- Timing: bench/results/std-twin-rows10-2026-11-03.md (sort 0.82, steady 0.69, fix 1.02 of container/heap's wall)

`heap.new`, `tracked`, `from_list`, `push`, `pop`, `peek`, `get`, `remove`, `fix`, `len`, `is_empty`, `to_list` (spec/std.md 56). The comparison is a function, and `moved(x, i)` stands for Go's `Swap` bookkeeping.
