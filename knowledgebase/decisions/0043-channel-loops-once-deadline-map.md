# 0043: Channel loops, task.Once, deadline map_concurrent

Status: Accepted (2026-10-08)

## Context
Stage 2 and 3 subjects wrote worker loops as `while { match ch.recv() { ... } }`,
hand built "many waiters, one result" from a closed channel plus a mutex
(07 cache), and could not keep finished results when a deadline hit
(`with_timeout` around `map_concurrent` loses them, 02 worker pool).

## Decision
- `for x in ch` receives until the channel is closed and drained.
- `task.once[T, E]()` and `Once.get(f)`: single flight with shared result.
- `task.map_concurrent_until(clock, d, items, limit, f)`: unfinished items give
  `Err(TimedOut)`, finished ones are kept, in input order.
