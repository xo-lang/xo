# 0063: Mutex.with_read, a shared read mode with writer preference

Status: Accepted (2026-10-08)

## Context
After decision 0047 the store mutex of bench/perf/http (task 01) was
still the top blocking site at 32 workers: every request, including the
two GETs and the occasional list of each cycle, took one exclusive
`Mutex.with`. The Go solution uses a `sync.RWMutex` for its reads
(`bench/results/perf-throughput-2026-10-08.md`). Xo had no way to say
"read only, may share", although `with(fn(m) ...)` without `var` already
gave a read only view (core 5.4).

## Decision
- New prelude method
  `fn (m: Mutex[T]) with_read[R, E, fx: Effects](f: fn(T) -> R ! E uses fx) -> R ! E uses fx`.
  The same shape as `with` (errors and effects pass through), but `f`
  takes `T` by value, so the existing mutation checks reject changes
  (`XO0501`, `XO0502`) and nothing is written back. A closure parameter
  declared `var` where the expected function type takes it by value is
  now error `XO0401` (it changed a copy, and the Go backend miscompiled
  it); this also closes the hole for every other `fn(T)` parameter.
- Many readers or one writer, in one runtime lock (`std/rt/sync.go`): the
  mutex is held exclusively while `permits` is 0 and shared while
  `readers` > 0.
- Writer preference: while a writer waits (or was woken and retries), a
  new reader queues. A release wakes the head of the queue and, if that
  is a reader, the readers right behind it; a woken reader may enter
  past later writers because it was queued ahead of them.
- Real runs barge as in decision 0047 (woken waiters compete again,
  `runtime.Gosched` after a wake, handoff after 4 lost races). Under
  `xo test`, `--seed`, recording, and replay the lock hands the permit
  over in FIFO order, readers at the head together. Readers inside
  together interleave at scheduling points, so `--explore` enumerates
  them like any tasks.
- Re-entrant use in one task (`with` or `with_read` inside either on the
  same mutex) faults `XO-F004`: a nested read would deadlock behind a
  queued writer.
- XO0804 (split critical section) treats a value read in `with_read` as
  a source like one read in `with`; a later `with_read` is not an act.
- Deadlock reports name the readers holding a mutex ("held for reading by
  task 2; writers waiting: task 4") and the wait as `Mutex.with_read`.

## Consequences
- Reads of shared state no longer serialize. In bench/perf/http the
  store's `get` and `list` use `with_read`; results in
  `bench/results/perf-throughput-2026-10-08.md` (section dated
  2026-10-08, "Shared reads"): no measurable throughput change there,
  because three of the five requests per cycle write and writer
  preference queues the GETs behind them; the write side is the next
  cost.
- A reader that waits for another reader to enter can deadlock when a
  writer queues in between (as with Go's RWMutex). The deterministic
  scheduler finds it under some seeds and reports XO-F001
  (`TestSchedReaderWaitsForReader`); the spec says so (std.md 2.7).
- `Mutex` keeps one type: no separate `RwLock`, so changing a read site
  from `with` to `with_read` is a one word edit.

## Alternatives
- A separate `RwLock[T]` std type: two types for one concept, and every
  existing `Mutex` would need a type change to gain shared reads.
- Reader preference (no writer queueing): simpler, but a steady stream of
  GETs starves writers under load.
- Building on Go's `sync.RWMutex`: its waits cannot be canceled (core 7.2
  makes the wait a cancellation point) and it cannot run FIFO under the
  deterministic scheduler.
- Making plain `with(fn(m) ...)` without `var` take the shared mode
  implicitly: changes the meaning of existing code (a reader closure that
  waits on a writer would start deadlocking) and hides the mode.
