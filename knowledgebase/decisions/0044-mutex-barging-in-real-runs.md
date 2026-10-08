# 0044: Mutex and semaphore waits barge in real runs, FIFO under test

Status: Accepted (2026-10-08)

## Context
The runtime lock behind `Mutex`, semaphores, and the std/sql pool
(`std/rt/sync.go`) was strictly FIFO: release handed the permit to the
first waiter. Under HTTP load (bench/perf/http, 32 workers) the store
mutex was held about 2 us per acquisition but waiters waited about 375 us:
the permit sat unused until the woken goroutine ran, and Go puts a woken
goroutine in the waker's runnext slot, so it waits for the waker to block
(often the rest of a request). Throughput was capped near 70k req/s with
the lock busy about 14 percent of the time.
(`bench/results/perf-throughput-2026-10-08.md`,
`gotchas/lock-convoy-runnext-wake.md`.) The spec (core 5.4) promises no
fairness order for `Mutex.with`.

## Decision
- In a real run (no test world, no deterministic scheduler), release frees
  the permit and wakes the first waiter, which competes for it again: a
  running task may take it first (barging, as Go's sync.Mutex). The
  releaser then calls `runtime.Gosched` so the woken goroutine runs now.
- A waiter that loses 4 times in a row asks for handoff (`starving`); the
  next release hands it the permit directly.
- Under `xo test` worlds, `XO_SEED`, recording, and replay the lock stays
  strictly FIFO with handoff, so schedules stay deterministic.

## Consequences
- 32 worker throughput of task 01 under `--arenas` went from 64k to 88k
  req/s (medians of 5, `bench/perf/http/run.sh`), p99 from 922 to 1111 us.
  The store mutex is still the top blocking site, with about 7 waiting
  goroutines instead of 22.
- Waiters in real runs can be overtaken (bounded by the starvation
  switch). Programs must not rely on FIFO acquisition order; the spec never
  promised it.

## Alternatives
- FIFO handoff plus `runtime.Gosched` after waking: 92k req/s against 107k
  for barging in the same session; kept for test runs only.
- Spinning before parking, with `runtime.Gosched` (puts the spinner on the
  global run queue) or a timed busy loop (32 spinners starve the holder of
  a processor): both cut throughput to 11k to 69k req/s and raised p99 to
  tens of ms. Rejected (`gotchas/lock-spin-starves-holder.md`).
- Building the real run lock on sync.Mutex: no way to cancel a waiter
  (core 7.2 makes the wait a cancellation point).
