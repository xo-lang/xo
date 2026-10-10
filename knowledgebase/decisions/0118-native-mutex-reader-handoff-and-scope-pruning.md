# 0118: Native Mutex hands the lock to waiting readers; long lived scopes drop finished tasks

Status: Implemented (2026-10-10). Amends 0082 point 1 (who a release
wakes) and the scope task list of 0068. Code: `std/llrt/xo_rt.c`
(`mutex_wake`, `mutex_grant_readers`, `xo_mutex_rlock`, `scope_prune`,
`scope_drop_pruned`, `xo_task.gone`), `std/llrt/xo_http.c` (the server's
connection scope). Tests: the `conc_rwlock_handoff` golden (TestGolden,
TestGoldenTasks under ThreadSanitizer with 1, 2, and per CPU workers,
arm64 TestGoldenRunShared), the other `conc_*` and `http_*` goldens.
Numbers: `bench/results/native-runtime-2026-10-10.md`, section 2.

## Context

Decision 0082 made a Mutex release wake one writer, or every reader when
no writer waits; a woken waiter checks again. The concurrency benchmarks'
only Mutex workload (8 tasks adding under `with`) hardly waits, so a
readers and writer workload was added (`bench/perf/conc` `rwmutex`: 64
tasks, 4,000 accesses each, a quarter of them writes holding the lock
for about 111 Collatz steps). Natively it took 967 ms and 6.3 s of CPU
(Go 11 ms, the Go backend 375 ms) with 912,000 fiber parks for 256,000
accesses: a writer's release woke the readers, but by the time they ran
another writer had queued (writers are preferred), so they parked again.

The TLS benchmark (decision 0117) showed the server's resident memory
growing about 1 KB per connection. A scope keeps every task it started
in its list until it closes (`xo_scope_close` releases them), and the
native HTTP server's connection tasks live in one scope as long as the
server.

## Decision

1. **Phase fair handoff to readers**, as Go's RWMutex: a writer's release
   hands the lock to every waiting reader at once (each counted in
   `readers`, taken off the wait list, marked granted, then woken); a
   granted reader returns without taking the Mutex's lock again or
   checking. The next writer waits for them; the last reader's release
   wakes the oldest writer (still woken, not handed the lock: a writer
   may barge, as in 0082). A reader canceled while it waits unwinds,
   unless the lock was handed to it first, in which case it holds a
   share as usual and its next cancellation point unwinds.
2. **Spinning before parking was tried and not kept**: a bounded spin
   (50 to 3,000 rounds of a CPU pause, reading the state without the
   lock) made `rwmutex` slower at every length (264 ms of CPU without,
   274 to 1,165 ms with) and did not change `mutex`.
3. **A scope drops the tasks that are over and unreferenced.** A task
   records when its run ended and when its queue entry was taken
   (`gone`, the last touch of either); a spawn that finds the scope's
   list doubled since the last time (at least 64) takes out the tasks
   with both and a count of one (the list's), unlinks their nodes, and
   releases them outside the scope's lock. The join never prunes (its
   helpers walk the list without the lock once it starts).

## Consequences

- `rwmutex`: 967 to 131 ms wall, 6,262 to 253 ms CPU (llvm), 912,000 to
  230,000 fiber parks; still about 12 times Go's wall time (Go's
  goroutines park less: every remaining native park is a reader behind a
  queued writer). The other concurrency workloads do not change.
- bench/perf/http (kept alive connections, users CRUD, its store uses
  `with_read`): throughput +1 percent, CPU per request -3.5 percent, p50
  240 to 195 us, p99 870 to 955 us.
- A server's finished connection tasks no longer accumulate: peak RSS
  after 20 s of a TLS handshake per request 295 to 223 MB. The rest of
  the growth is the allocator (blocks freed on another thread than the
  one that allocated them stay on the freeing thread's list).
- A failed task whose handle was dropped is pruned like any other; the
  scope keeps its first failure and fault by their own references.

## Alternatives

- **Handoff to writers too**: no barging, but the lock then waits for
  the woken fiber to be scheduled (gotchas/lock-convoy-runnext-wake.md);
  readers were the measured problem.
- **Readers first always** (no writer preference): readers would starve
  writers under a read heavy load.
- **Pruning at every task end**: the ending task would take the scope's
  lock, which 0077 removed from the task path; pruning at spawn is
  amortized and off the workers.
