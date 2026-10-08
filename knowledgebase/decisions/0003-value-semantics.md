# 0003: Value semantics, no pointers

Status: Accepted (2026-10-08). Zero value semantics violations in every benchmark run; data races were inexpressible or rejected for 5 of 5 planted races (author 1). The friction it caused (map entry updates, collecting worker results, background work) was removed by `Map.update`, task groups, and `map_concurrent` (0016, 0032); the performance cost was removed by in place writes for unshared Map and List (perf-map, perf-list results).

## Context
Data races and aliasing bugs are among the hardest for agents to debug. A full
borrow checker is too heavy for a Go like language.

## Decision
All types are values. Builtin collections are immutable persistent structures.
Mutation happens only through `var` bindings and `var` receivers (in out
semantics). In place mutable buffers (`Buf[T]`, `StrBuilder`) are move only.
Shared mutable state requires `Mutex[T]` or `Atomic[T]`.

## Consequences
- Data race freedom without lifetimes.
- Persistent collections are slower than Go slices for hot loops; `Buf[T]` is
  the escape hatch. Benchmark early.
- Graph structures need ids into maps, not pointers.

## Alternatives
- Go style pointers plus a race detector: races found only at runtime.
- Rust style borrowing: too much complexity and too many compile iterations.

## Evidence
- Paper test 2026-10-07 ([results](../../bench/results/paper-2026-10-07.md)): 0 violations, high friction in 4 of 5 tasks (get, modify, put; lost update risk).
- Paper test 2026-10-07 spec 0.2 ([results](../../bench/results/paper-2026-10-07-v0.2.md)): 0.2 run: friction persists in 2 of 5 tasks (worker results, atomic take plus compute), 0 violations.
- Paper test 2026-10-07 spec 0.3 ([results](../../bench/results/paper-2026-10-07-v0.3.md)): 0.3 run: third run in a row with friction in concurrent code (02, 05); 0 violations.
- Planted bugs 2026-10-07: value semantics would reject all 5 planted data races and 3 value receiver lost updates (estimate). ([results](../../bench/results/planted-and-harder-2026-10-07.md))
- Planted bugs in Xo 2026-10-07: data races inexpressible or rejected, but check then act across two `Mutex.with` sections escaped twice (atomicity, not races). ([results](../../bench/results/planted-xo-2026-10-07.md))
