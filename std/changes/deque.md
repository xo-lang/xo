# std/deque changes

## 2026-10-30: feature: std/deque

- Kind: feature
- Commit: this commit
- Found by: Xo Lang 1's triage of the 2026-10-30 language obstacles note
  (a work queue popped from the front copied the list each time)
- Go reference: container/list used as a queue, a slice ring buffer
- Test: internal/stdtest/testdata/corpus/deque (a Go slice model's
  answers to 480 operations across growth and wrap around), every backend
- Timing: see the row below (bench/perf/deque against Go's
  container/list and a slice queue)

`deque.new()` makes a double ended queue with O(1) pushes and pops at
both ends.

| workload (20,000,000 ops) | Xo native (s) | Go container/list (s) | Xo / Go |
|---|---|---|---|
| queue (push_back, pop_front) | 0.074 | 0.419 | 0.18 |
| both (alternating ends) | 0.051 | 0.272 | 0.19 |

darwin/arm64 (M-series Mac), 2026-10-30, medians of 5 interleaved runs
less the `none` workload (setup only, 0.008 s both sides); twincheck
agrees on every checksum. container/list allocates a node per element,
the ring none per element after growth.
