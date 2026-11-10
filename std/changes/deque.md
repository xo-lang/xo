# std/deque changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus deque on go, arm64 and llvm

std/deque.xo now holds the bodies of std/native/deque.xo (removed), and the Go
backend compiles them from it as its own package, in place of the hand written std/rt/deque.go (removed): the Go
backend now runs the same deque code as the native backends. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-05: perf: the ring is allocated once at its size

- Kind: perf
- Commit: this commit
- Found by: the capacity hint audit after 1b4b2bfa1 (xo-lang-4)
- Go reference: make([]T, n) for a ring
- Test: internal/stdtest corpus deque, every backend
- Timing: 200,000 push_back, linux/arm64 0.614 from new(0), 0.741 from new(200_000); darwin/arm64 0.844, 0.765 (bench/results/capacity-hints-2026-11-05.md)
- Shipped in: not yet released

On the native backends `deque.new(cap)` and each growth built the new ring
by pushing onto an empty list, so the ring itself grew through about
log2(size) copies; it is now allocated once at its size.

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
