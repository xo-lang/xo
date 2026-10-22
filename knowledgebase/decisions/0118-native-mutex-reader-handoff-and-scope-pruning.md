# 0118: Native Mutex hands the lock to waiting readers; finished tasks and freed blocks do not pile up

Status: Implemented (2026-10-10). Amends 0082 point 1 (who a release
wakes) and the scope task list of 0068. Code: `std/llrt/xo_rt.c`
(`mutex_wake`, `mutex_grant_readers`, `xo_mutex_rlock`, `scope_prune`,
`scope_drop_pruned`, `xo_task.gone`, `alloc_mt`, `free_mt`,
`central_put`, `central_take`), `std/llrt/xo_http.c` (the server's
connection scope). Tests: the `conc_rwlock_handoff` golden (TestGolden,
TestGoldenTasks under ThreadSanitizer with 1, 2, and per CPU workers,
arm64 TestGoldenRunShared), the other `conc_*` and `http_*` goldens.
Numbers: `bench/results/native-runtime-2026-10-10.md`, section 2.
Since 2026-10-10 also std/llrt/testdata/tasks.c `handoff`, `prune`, and
`magazines` (TestRuntimeTasks; `xo_chunks` counts carved chunks).

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
4. **Freed blocks move between threads in magazines.** The allocator put
   a block freed on another thread on the freeing thread's list, so a
   thread that allocates what others free (an accept loop, a spawning
   loop) carved new 4 MiB chunks for good. With threads, each heap keeps
   per size class its free list and one full spare magazine (64 KiB of
   blocks, at least 8); a list that reaches a magazine's size becomes the
   spare and the old spare goes to a central stack per class (one
   mutex); an empty list takes the spare, else a central magazine, before
   it carves. O(1) per magazine, nothing walks a list; the single
   threaded paths only count.

## Consequences

- `rwmutex`: 967 to 131 ms wall, 6,262 to 253 ms CPU (llvm), 912,000 to
  230,000 fiber parks; still about 12 times Go's wall time (Go's
  goroutines park less: every remaining native park is a reader behind a
  queued writer). The other concurrency workloads do not change.
- bench/perf/http (kept alive connections, users CRUD, its store uses
  `with_read`): throughput +1 percent, CPU per request -3.5 percent, p50
  240 to 195 us, p99 870 to 955 us.
- A server's finished connection tasks no longer accumulate, and freed
  blocks come back: peak RSS after 20 s of a TLS handshake per request
  295 MB, 223 MB with pruning, 28 MB with magazines too, at the same
  rate. bench/perf/http kept alive: RSS 30 to 21 MB, throughput the same.
- Reusing blocks that other cores wrote costs where one thread spawns
  and others finish: `spawn` 15 percent and `fanout` 24 percent more
  wall time (interleaved, 9 rounds) than carving fresh memory, whose
  peak RSS grew without bound (45 and 24 MB for these short runs, 5 MB
  now). Accepted: bounded memory for long running programs.
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
- **Returning a remote free to the allocating heap** (mimalloc's
  delayed free lists): needs the owner of every block (aligned chunks
  with a header) and an atomic list per heap; magazines needed neither.
- **Moving half a list when it grows past a limit**: tried first; the
  walk to split the list plus the lock made `spawn` 30 and `fanout` 43
  percent slower, magazines 15 and 24.

## Addendum 2026-10-22: a release yields to the writer it wakes

Status: Implemented (owner, 2026-10-22: ship as the default). Writer
preference and spec/std.md 2.7 are unchanged. Code: std/native/llrt
mutex.xo `wake` (returns whether to yield), `unlock_body`, `reader_out`,
`M_READ`; pool.xo `mutex_yield`, `yield_now`, `run_item` (W_ASYNC 2),
`DLK_MNOYIELD`; poolsetup.xo `XO_MUTEX_YIELD`.

Context. The CRUD server (bench/perf/http) parked a task on its store
Mutex about 0.9 times per request, on 1 core as on 6 (XO_RTSTATS
sig_mutex), with or without the list endpoint's long read. A release woke
the next waiting writer into the ring (decision 0082's `mutex_signal`).
Until that writer ran, the Mutex was free, yet the waiting writer kept the
writer bit set, so every reader that arrived parked behind it. Each park
needs a wake, and the pattern fed itself: about one park per operation. A
micro test (512 tasks on 4 threads, 1 write to 3 reads, tiny sections)
parked 102k times for 102k operations, and none on 1 thread. The Go
backend had the same convoy on 2026-10-08
(gotchas/lock-convoy-runnext-wake.md) and fixed it with
`runtime.Gosched()` after the wake.

Decision. A release that wakes a writer (a writer's unlock, or the last
reader's) puts it in the worker's next and yields the releasing fiber to
the back of the worker's own ring (run_item, W_ASYNC 2), so the woken
writer runs at once on this thread and the releaser right after this
worker's queued work. Only on a Mutex that `with_read` has taken (M_READ):
the convoy is readers queued behind a woken writer, and on a writers only
Mutex the yield bought nothing and cost a task switch per release. Not
with one CPU (the pool's P_NCPU), where the yield only delays the
releaser. `XO_MUTEX_YIELD=0` turns it off. Reader grants (grant_readers)
are unchanged.

Numbers. CRUD linux/arm64 in Docker, interleaved, quiet windows; old
release against this one:
| cores | rps | CPU us/req | p99 ms |
|---|---|---|---|
| 1 | 101.0k to 103.1k (no yield with one CPU) | 12.8 to 12.6 | 23.3 to 22.2 |
| 3 | 88.6k to 190.8k (2.15x) | 24.9 to 17.0 | 53.8 to 15.6 |
| 6 | 70.5k to 136.0k (1.93x) | 37.5 to 27.1 | 65.7 to 27.0 |
(3 and 6 cores: 2 reps, agents/myq4-cmd.log; 1 core: 4 reps of the same
rule, agents/myq-cmd.log. The first form, yielding to the inject queue's
tail on every Mutex, gave 203.9k and 161.8k at 3 and 6 cores, but
bench/perf/conc mutex 3.4x and rwmutex 3.7x the CPU.)

bench/perf/conc on the Mac (3 interleaved rounds, agents/myq4-cmd.log,
myq5-cmd.log): mutex CPU 21.0 to 21.1 ms before, 20.5 to 21.5 after
(unchanged); spawn unchanged; rwmutex, a loop of tasks that only lock and
read, wall 41.8 to 44.5 ms before, 51.6 to 53.8 after (+19 to 24%), CPU
109 to 120 ms before, 140 to 157 after (+19 to 28%). This is the known
cost: the next experiment yields only when a reader actually parked
behind the signaled writer.

Writer starvation (8 readers re-entering with_read with no gap, about 35
us each, one writer timing 200 `with` waits; Mac, 4 and 8 threads, 3 runs
each): before p50 35 to 41 us, max 50 to 180 us; with the yield p50 0 to
35 us, max 45 to 81 us. Writer preference holds as before.

Cost when uncontended: no waiter, no yield (1 thread: no parks and the
same time either way, 0.023 to 0.035 s for 25.6k operations).

Alternatives measured (not pushed):
- Readers pass a waiting writer while no writer holds the Mutex, bounded
  at 1 ms of the oldest writer's wait (Go's starvation bound): parks -97%
  on the micro test, CRUD 1.45x and 1.49x at 3 and 6 cores, but a spec
  change (std.md 2.7), and under a continuous read stream the writer then
  waited its full 1 ms every time (p50 1020 us).
- Hand the Mutex to the woken writer at the release (owned, never free):
  no gain, since readers still queue behind the writer's wake (CRUD 0.96x
  at 6 cores).
- Spin before parking (Go's active spin): it never spun, because Go's
  canSpin rule refuses whenever the worker has queued work, which with
  many tasks is always; no gain.
