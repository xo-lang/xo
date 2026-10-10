# 0082: Native Mutex moves its value into `with`, wakes only who can enter; Map.remove keeps dead values until a rebuild

Status: Accepted (2026-10-09); point 1 amended by 0118 (2026-10-10: a
writer's release hands the lock to the waiting readers). Amends decision 0072 point 3 (removal) and
the Mutex of decision 0068 (waking); the semantics of core 5.4 are
unchanged. Implemented in `std/llrt/xo_rt.c` (`mutex_wake`,
`mutex_signal`, `wl_wait_as`), `internal/ssa/build_conc.go`
(`mutexWith`), `internal/ssa/build.go` (`packedValue`),
`internal/ssa/build_map.go` (`mapRemove`). Evidence:
`bench/results/perf-throughput-2026-10-08.md`, "Native backends";
`gotchas/native-mutex-write-copies-value.md`.

## Context
The first native HTTP throughput run (bench/perf/http, 32 workers) gave
64k req/s against 123k for Go and 134k for the Go backend. The store's
`Mutex[State]` was the cause: every release woke every waiter, every
`with(fn(var st) ...)` copied the whole map (the mutex's list still held
the value, so the closure's first change saw a count of 2), each copy
updated an atomic count per string of every user, and the store's delete
(`if st.users.remove(id) is None { return Err(...) }`) copied the values
because remove wrote zero values into them.

## Decision
1. **Wake only who can enter.** Waiters queue in arrival order with their
   kind. A release wakes the oldest waiting writer when the mutex is free
   and a writer waits, else (no writer waits) every waiting reader; a
   reader's release wakes a writer when it was the last reader. A woken
   waiter checks again and waits again if another context took the lock
   (barging); whoever holds it wakes the next one at its release, and a
   canceled writer passes the wake on.
2. **Mutex wake ups go to the ring.** The woken fiber goes to the waking
   thread's run ring, not its `next` slot (decision 0077 point 2), so a
   spinning worker runs it while the waker finishes its own work.
   Channel and task wake ups keep `next`.
3. **`with` with a `var` closure moves the value.** The value leaves the
   mutex's list (`list.pop`) and is the closure's to change in place. The
   closure returns the value the mutex keeps: the changed one on success,
   the one it was given when it returns an error (statically, or decided
   by a Result tail at run time) or unwinds. Perceus then keeps a second
   reference, and the first change copies, only when a failure can follow
   a change; otherwise the update is in place.
4. **`Map.remove` clears only the live flag.** The removed entry's key and
   value stay in their lists, unreachable, until the next rebuild (which
   already happens when live and dead entries pass three quarters of the
   table) drops them. Removing from a shared map copies the flags list,
   not the values.

## Consequences
- bench/perf/http on arm64: these four steps took it from 64k to about
  130k req/s and p99 from 1,788 to about 800 us; with the other fixes of
  the same day, 142k (Go 121k, the Go backend 134k), p99 670 us (each
  step in the results). bench/perf/conc `mutex` (8 tasks x 25,000
  `with`): 52 to 12 ms wall, 260 to 20 ms CPU on llvm; 59 to 10 ms on
  arm64 (the Go backend takes 115 ms).
- A removed value lives until the next rebuild: at most about three
  quarters of the table's size in dead entries, as many as removes since
  the last rebuild.
- A closure that changes the value and may fail after the change still
  copies on its first change (the rollback needs the old value).
- The wake order is FIFO per kind, but barging means a newcomer can take
  a free lock before the woken waiter runs; starvation is not bounded
  (the Go backend's runtime lock switches to handoff after 4 losses;
  not needed in the measurements so far).

## Alternatives
- **Waking every waiter** (the earlier code): simple, and correct with
  barging, but 32 waiters made a thundering herd on every release.
- **Handoff** (the lock passed to the woken waiter): no barging, but the
  lock waits for the woken fiber to be scheduled (gotchas
  lock-convoy-runnext-wake.md).
- **A static check that no failure follows a change** (an AST pass over
  the closure): decides before the body is built but must model every
  unwinding call; returning the given value on failure and letting
  Perceus decide needs no such model.
- **A persistent map** (the Go backend's log structure): O(1) copies,
  but a new structure for both native backends; the move made copies
  rare instead.
- **Non blocking socket polls by spinning workers** (Go's netpoll(0) in
  findrunnable): measured, 2 percent more requests and 5 percent less
  CPU, but p99 3 to 10 percent worse; not kept (results, "Native
  backends").
