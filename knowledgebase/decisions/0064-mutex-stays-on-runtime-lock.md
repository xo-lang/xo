# 0064: Real run Mutex stays on the runtime lock, not sync.RWMutex

Status: Accepted (2026-10-08)

## Context
In bench/perf/http (task 01) the Go and Xo servers use the same lock
design, yet Xo spent far more time blocked on the store lock. One
candidate was the lock itself: Xo's `Mutex` is the runtime lock of
`std/rt/sync.go` (explicit waiter queue, barging in real runs, decisions
0047 and 0063), Go's is `sync.RWMutex` (spinning, runtime semaphores).
Decisions 0047 and 0063 rejected sync.Mutex/RWMutex because a waiter
cannot be canceled (core 7.2) and cannot run FIFO under the
deterministic scheduler; the open question was whether the gain in real
runs would justify a hybrid (sync lock when no cancellation can arrive,
or a try lock plus a cancelable slow path).

Measured first (`bench/results/perf-throughput-2026-10-08.md`, "Lock hold
time"): the waits came from hold time. Xo held the lock 8 to 30 times
longer than Go; after the runtime and codegen fixes of that section,
Xo's wait p50 is 0.3 us like Go's and its mean wait (8 to 12 us) is below
Go's (5 to 20 us).

Then the upper bound of the lock swap: a prototype that backed
`Mutex.with` with `sync.RWMutex.Lock` and `with_read` with `RLock` in real
runs, ignoring cancellation and deadlock reports entirely. Five
interleaved runs against the runtime lock on the same build: 62.0k
against 63.2k req/s without arenas and 101.3k against 100.0k with arenas
(medians; per run ranges overlap fully), p99 1786 against 1752 us and
1158 against 1139 us. Writer waits under sync.RWMutex were worse (mean
91 us against 8 us with lock stats on): its writers wait behind every
reader that entered first, while the runtime lock queues new readers
behind a waiting writer and lets woken waiters barge.

## Decision
- Real runs keep the runtime lock (barging, `runtime.Gosched` after a
  wake, handoff after 4 lost races; decision 0047), with shared reads as
  in decision 0063. Test worlds, `XO_SEED`, recording, and replay keep
  strict FIFO handoff. Every guarantee of 0047 and 0063 holds: the wait
  is a cancellation point, deadlock reports name holders and waiters.
- The fixed cost moves out of the critical section instead: the holding
  site (stack walk) and the task's held map entry are recorded before the
  wait, and the owner is set in the same internal critical section that
  takes the permit (`holdSite`, `acquireMode(tk, shared, site)`). A wait
  that is canceled removes its held entry.

## Consequences
- No hybrid lock: one implementation, cancelable everywhere, no mode
  that changes behavior depending on whether a task can be canceled.
- The prototype is not kept; repeating it means adding a `sync.RWMutex`
  field to `Mutex` and a branch in `Mutex_with` and `Mutex_with_read`
  when `barging(tk)` (the measurement procedure is in the results file).
- If a future workload shows the runtime lock itself as the cost (hold
  times near Go's but waits far longer), revisit with a lock free fast
  path in the runtime lock (an atomic state word for the uncontended
  acquire and release, the queue only for waiters), which keeps
  cancellation.

## Alternatives
- sync.RWMutex in real runs, with cancellation through a helper
  goroutine that takes the lock and hands it over: the lock would sit
  owned while the task is woken, the convoy of
  `gotchas/lock-convoy-runnext-wake.md`; and the upper bound above shows
  no gain to pay for it.
- TryLock fast path on a sync.Mutex plus the runtime queue as the slow
  path: two locks that must agree on who holds the mutex; not sound
  without making the slow path take the sync lock, which brings back the
  uncancelable wait.
- sync lock only for tasks that cannot be canceled: in a server every
  request task can be canceled (shutdown), so it would never apply where
  it matters.
