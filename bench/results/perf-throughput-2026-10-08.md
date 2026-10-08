# Request throughput: task 01 under HTTP load (2026-10-08)

Goal: close the request throughput gap to Go measured in
`perf-arenas-2026-10-08.md` (Go 123k req/s, Xo `--arenas` 73k at 32
workers). Planned work: direct JSON codecs for `derive Json`, a cheaper
per request task path, profiles before and after.

Host: darwin/arm64 (M4 Pro laptop, 12 cores), Go 1.26.1. The host was
shared with other workloads (load average 15 to 30 during the runs); the
first round of each run below is visibly disturbed, the medians of 5 are
not. Same harness and settings as the earlier results:
`bench/perf/http/run.sh`, 32 workers, 2 s warmup, 10 s measured, 64
preloaded users, loopback, configs interleaved.

## What the profile said

Profiles come from the generated server under the same load (5 s,
`XO_CPUPROFILE` and `XO_BLOCKPROFILE`, new in this change, see
`knowledgebase/infrastructure/toolchain.md`).

Before (Xo `--arenas`, 71k req/s in the profiling run):

- CPU: `syscall.rawsyscalln` 44 percent, `runtime.usleep` 21,
  `runtime.kevent` 11, `runtime.pthread_cond_wait` 9. All Xo code
  (handlers, router, JSON, store) was 5 percent of samples, and the
  server used only 3.3 of 12 cores.
- Block: 96 percent of blocked time in `runtime.selectgo` under
  `(*lock).acquire` from `Mutex_with` (the store mutex), about 22 of the
  32 request goroutines waiting at any time.
- Instrumented: the store mutex was held 2.2 us per acquisition (busy
  about 14 percent of the time) while an acquire waited 375 us on
  average.

So the gap was not CPU spent in Xo code or JSON: it was a lock convoy.
The runtime lock handed the permit to the first waiter; Go readies the
woken goroutine in the waker's runnext slot, where it waits until the
waker blocks (usually after the response is written), and the permit
sits owned but unused meanwhile
(`knowledgebase/gotchas/lock-convoy-runnext-wake.md`).

After (Xo `--arenas`, 107k req/s in the profiling run):

- CPU: `syscall.rawsyscalln` 66 percent, `runtime.usleep` 13,
  `runtime.pthread_cond_wait` 6, `runtime.kevent` 4; the server uses 4.7
  cores. Xo code is still about 4 percent of samples (`Mutex_with` 1.0,
  the create handler 0.7, `email_taken` 0.5).
- Block: `(*lock).acquire` is still the top entry but at about 7 waiting
  goroutines instead of 22.

## Changes

1. Runtime lock (`std/rt/sync.go`, decision 0047): in real runs release
   frees the permit and wakes the first waiter, which competes again
   (barging), then calls `runtime.Gosched` so the woken goroutine runs at
   once; a waiter that loses 4 times gets handoff. Test worlds, `XO_SEED`,
   recording, and replay keep strict FIFO handoff.
2. Direct JSON (`internal/gen/json.go`, `std/rt/jsonfast.go`):
   `derive Json` structs get `XoJsonEnc` with precomputed key text and
   typed writers (`w.Str`, `w.Int`, `w.Bool`; no boxing, no object
   stack) and a new `XoJsonRead` that decodes straight from the bytes
   (key switch, unknown keys skipped, defaults for missing fields).
   `json.decode` checks `json.Valid`, runs the direct decoder, and on any
   failure reruns the tree decoder, so error messages and paths are
   unchanged. `rt.JsonEnc` no longer boxes its argument (type switch on
   a pointer). Golden tests: `internal/gen/testdata/json_direct.golden`
   (generated code) and `testdata/golden/json_fast` (behavior, checked
   against a run with the fast path disabled).
3. Per request task: the cancellation node's `done` channel is made on
   first use (`cnode.doneCh`), one allocation less per request and per
   task; most request tasks are never waited on.

Items 2 and 3 were not measured separately: Xo code is about 4 percent of
CPU samples, so they cannot move throughput much on this workload. Item
1 is the throughput change.

Rejected on the way (`knowledgebase/gotchas/lock-spin-starves-holder.md`):
spinning before parking, with `runtime.Gosched` (25k req/s) or a 20 us
busy loop (11k req/s); FIFO handoff plus `Gosched` (92k against 107k for
barging in back to back runs).

## Results (32 workers, medians of 5)

Run 1, before and after with arenas (`CONFIGS="go xo-base-arenas
xo-arenas"`, `xo-base-arenas` is the xo binary of commit b09aa19):

| Config | Req/s | p50 us | p99 us | p99.9 us | GCs/10k req | GC CPU % |
|---|---|---|---|---|---|---|
| Go | 123,959 | 218 | 920 | 1532 | 24.0 | 4 |
| Xo --arenas before | 64,075 | 433 | 922 | 1307 | 87.3 | 6 |
| Xo --arenas after | 88,249 | 293 | 1111 | 1757 | 64.8 | 9 |

Run 2, without arenas (`CONFIGS="go xo-base xo xo-arenas"`):

| Config | Req/s | p50 us | p99 us | p99.9 us | GCs/10k req | GC CPU % |
|---|---|---|---|---|---|---|
| Go | 119,675 | 226 | 907 | 1422 | 22.8 | 4 |
| Xo before | 47,134 | 698 | 1129 | 1658 | 214.1 | 10 |
| Xo after | 50,873 | 639 | 1545 | 2072 | 233.4 | 16 |
| Xo --arenas after | 84,217 | 306 | 1115 | 1677 | 72.5 | 10 |

- With arenas, throughput rises 38 percent (64k to 88k) and p50 falls
  from 433 to 293 us; Xo reaches 0.71x Go (0.52x before in this session;
  the earlier report had 73k against 123k, 0.59x, on a quieter host).
- p99 rises (922 to 1111 us): barging lets a running request overtake a
  woken one; the starvation switch bounds it.
- Without arenas the gain is small (47k to 51k): that server is GC bound
  (16 percent GC CPU, 233 GCs per 10k requests), so removing the convoy
  moves the bottleneck to allocation.

## Next biggest cost

The store mutex is still the top blocking site after the fix (about 7
goroutines waiting on average). Go's solution uses an RWMutex (the two
GETs per cycle share it) and an email index; the Xo solution takes one
exclusive `Mutex.with` for every request and scans every user for the
email check under the lock. Next experiment: a shared read mode for
`Mutex` reads (or an `RwLock` std type) used by `Store.get` and
`Store.list`, then rerun `CONFIGS="go xo-arenas" sh
bench/perf/http/run.sh` with `XO_BLOCKPROFILE` on one run to see whether
the remaining wait moves to the network path. Without arenas the next
cost is allocation (GC CPU 16 percent).

## Shared reads: Mutex.with_read (2026-10-08)

Change: `Mutex.with_read` (decision 0063: many readers or one writer,
writer preference, barging in real runs) and `Store.get` and `Store.list`
of `bench/perf/http/xo` switched from `with` to `with_read`. Same harness
and settings as above: `REPS=5 CONFIGS="go xo xo-arenas" sh
bench/perf/http/run.sh`, 32 workers, 2 s warmup, 10 s measured, 64
preloaded users. Before is the xo binary of commit 05b84d1 on the old
store; after is commit f1185de. Host load average (1 minute) at the start
of each run: 3.75 before, 3.63 after (the runs themselves push it to
about 17).

| Config | Req/s before | Req/s after | p50 us before | p50 us after | p99 us before | p99 us after | GC CPU % after |
|---|---|---|---|---|---|---|---|
| Go | 122,732 | 123,993 | 220 | 219 | 919 | 911 | 4 |
| Xo | 51,908 | 50,690 | 630 | 643 | 1506 | 1526 | 16 |
| Xo --arenas | 84,716 | 84,939 | 303 | 306 | 1116 | 1088 | 10 |

No measurable change: every difference is inside the run to run spread
(Xo --arenas ranged 82k to 91k before and 82k to 89k after).

Why (block profile, `XO_BLOCKPROFILE`, Xo `--arenas` under the same load,
5 s, 107k req/s in the profiling run): `(*lock).acquireMode` is still 61
percent of blocked time, now split between `Mutex_with_read` from
`Store.get` (27 percent) and `Mutex_with` from create, update, and delete
(22 percent). Each load cycle is create, get, patch, get, delete: three
of five requests write. With 32 workers a writer is almost always waiting,
and writer preference queues new readers behind it, so the GETs still
take turns with the writes; readers only share the lock in the short
windows between writers. CPU: `syscall.rawsyscalln` 66 percent,
`runtime.usleep` 13, `pthread_cond_wait` 6; Xo code still about 2
percent (`email_taken` 0.8).

## Next biggest cost (after shared reads)

Still the store lock, now on the write side: writes are 60 percent of
requests and serialize, and each handoff between goroutines costs a wake
(woken goroutine scheduling) that is long against a 2 us hold. Next
experiments, in order of likelihood:

1. Shorten and split the writes: keep an email index (`Map[Str, Int]`) in
   `State` so `email_taken` is O(1) instead of a scan of every user under
   the write lock, and move `sort_by` in `list` out of the read section.
   Rerun `REPS=5 CONFIGS="go xo-arenas" sh bench/perf/http/run.sh`.
2. Shard the store by id (`List[Mutex[Map[Int, User]]]`, email index in
   its own mutex) so writes to different users do not contend; this is
   what removes the serialization rather than shortening it.
3. In the runtime lock, measure wake to run latency
   (`XO_BLOCKPROFILE` plus a counter of lost races per acquire) and try
   letting a woken reader batch include readers that arrive while the
   batch is waking.

## Lock hold time: why Xo waits longer on the same lock design (2026-10-08)

Question: Go and Xo use the same lock design on this workload (an
RWMutex, reads shared, the list sorted inside the read section), yet the
store lock was 61 percent of Xo's blocked time. Is it hold time or the
lock implementation?

Instrument (off by default): `XO_LOCKSTATS=<file>` in the Xo runtime and
`HOLDSTATS=<file>` in the Go server record, per operation, the wait to
acquire and the time inside the critical section
(`knowledgebase/infrastructure/toolchain.md`). One 10 s run per config
after 2 s warmup, 32 workers, 64 preloaded users, same harness as
`run.sh`. The samples stay in memory and change GC pacing, so these runs'
req/s are not comparable with `run.sh`
(`gotchas/sample-recording-changes-gc-pacing.md`); only the times are.

### Experiment 1: hold time

Before (commit 45349e6; host load 4.4 at the start), microseconds:

| Operation | Go wait p50 | Go wait p99 | Go hold p50 | Go hold p99 | Xo wait p50 | Xo wait p99 | Xo hold p50 | Xo hold p99 | Xo arenas hold p50 | Xo arenas hold p99 |
|---|---|---|---|---|---|---|---|---|---|---|
| create | 0.21 | 213 | 0.17 | 0.75 | 22.3 | 530 | 5.79 | 80.5 | 5.62 | 32.2 |
| get (read) | 0.17 | 67 | 0.17 | 0.50 | 51.3 | 605 | 1.33 | 20.1 | 1.33 | 7.3 |
| list (read) | 0.17 | 67 | 6.79 | 46.5 | 46.4 | 604 | 6.21 | 204 | 5.79 | 86.5 |
| update | 0.21 | 217 | 0.21 | 0.71 | 22.4 | 537 | 5.75 | 74.7 | 5.62 | 32.1 |
| delete | 0.21 | 217 | 0.21 | 0.75 | 21.9 | 530 | 1.50 | 20.4 | 1.50 | 12.5 |

Mean waits: Go 4.8 (reads) to 20 us (writes); Xo 66 to 95 us; Xo
`--arenas` 43 to 65 us.

So Xo held the lock 7 to 28 times longer at p50 (the list is the
exception: Go also sorts inside its read lock). Splitting the hold
(a temporary timestamp between the runtime's setup and the closure):
about 1.2 us of every hold was runtime bookkeeping after the permit was
taken (a 14 frame `runtime.Callers` walk for the holding site, the
task's held map insert, and a second lock of the lock's internal `l.mu`
to record the owner, which every waiter and releaser also hammers), and
about 3.7 us of create and update was the closure, mostly
`State.email_taken`: `st.users.values()` copied every user into a new
list, `Map.each` did a hash lookup per log entry on the churned map, and
`same_email` lowered both strings per user. The CPU profile showed only
about 0.8 us of CPU per scan; the wall clock hold was five times that
(`gotchas/lock-wait-is-hold-time-in-disguise.md`).

Fixes (commit 7bcb24d; no change to the benchmark source):

1. `Mutex.with`/`with_read`: the site walk and the held map insert run
   before the wait (`holdSite`); the owner and site are set in the same
   `l.mu` section that takes the permit; a canceled wait removes its
   entry (`TestMutexCanceledWaitForgetsSite`).
2. Codegen: inlined `any/all/find/fold/map/filter` and `for` loops over
   `m.values()` or `m.keys()` iterate the map in place
   (`rt.Map_values_seq`, `rt.Map_keys_seq`; golden `map_view_loops`
   matches the list form, value semantics included). A first version
   took `&v` for marking inside the per element closure, which moved
   every element to the heap (128 allocations per scan); types without
   lists now take a closure without it.
3. `Map.each` on a root table with overwrites and tombstones: two linear
   passes (superseded bitmap, current entry per origin) instead of a
   hash lookup per entry (`TestMapLargeChurnedLog`, the model tests).
4. `a.to_lower() == b.to_lower()` (and `!=`) lowers to
   `rt.Str_eq_lower`, which compares ASCII bytes lowered in place and
   falls back to `strings.ToLower` at the first non-ASCII byte
   (`TestStrEqLower`).

Micro benchmark of the scan on a churned 96 user map
(`BenchmarkStoreScanChurn`, std/rt): the list form cost about 3.0 us and
2 allocations (6.5 KB); in place with the four fixes about 1.3 us
including the churn step's own put, get, and remove.

After (commit 7bcb24d; host load 2.6 at the start), microseconds; Go
from the same session's run at 15:03 (load 2.0):

| Operation | Go wait p50 | Go wait p99 | Go hold p50 | Go hold p99 | Xo wait p50 | Xo wait p99 | Xo hold p50 | Xo hold p99 | Xo arenas hold p50 | Xo arenas hold p99 |
|---|---|---|---|---|---|---|---|---|---|---|
| create | 0.21 | 206 | 0.21 | 0.75 | 0.29 | 283 | 1.96 | 7.75 | 1.96 | 7.83 |
| get (read) | 0.17 | 65 | 0.17 | 0.50 | 0.29 | 314 | 0.21 | 1.00 | 0.21 | 0.92 |
| list (read) | 0.17 | 65 | 6.75 | 41.1 | 0.29 | 311 | 4.79 | 227 | 4.25 | 86.9 |
| update | 0.21 | 213 | 0.21 | 0.67 | 0.29 | 287 | 2.12 | 10.3 | 2.12 | 9.9 |
| delete | 0.21 | 210 | 0.21 | 0.75 | 0.29 | 283 | 0.38 | 6.17 | 0.38 | 5.50 |

Mean waits now: Xo 11.5 (writes) to 16 us (reads), Xo `--arenas` 7.8 to
12 us, against Go's 4.6 to 19.5 us. Get and delete hold about what Go
holds; create and update still hold about 2 us against 0.2 us because
the Xo store scans every user for the email check (Go keeps an email
index), which is the benchmark's design, not a codegen cost.

Throughput, `REPS=5 CONFIGS="go xo-base xo xo-base-arenas xo-arenas"
bench/perf/http/run.sh` with `xo-base` the xo binary of 45349e6 and the
others of 7bcb24d (load average 3.86 at the start, about 15 at the end):

| Config | Req/s | p50 us | p99 us | p99.9 us | GC CPU % | GCs/10k req |
|---|---|---|---|---|---|---|
| Go | 123,104 | 219 | 910 | 1470 | 4 | 24.7 |
| Xo before | 51,976 | 630 | 1500 | 1993 | 16 | 231.3 |
| Xo after | 61,034 | 427 | 1803 | 2517 | 15 | 172.8 |
| Xo --arenas before | 84,240 | 309 | 1088 | 1600 | 10 | 76.3 |
| Xo --arenas after | 95,137 | 271 | 1149 | 1818 | 10 | 58.0 |

Xo rises 17 percent and Xo `--arenas` 13 percent (ranges: Xo 50.9k to
52.1k before, 59.5k to 62.8k after; arenas 82.9k to 90.1k before, 94.1k
to 97.8k after), p50 falls, and p99 rises (1500 to 1803 us without
arenas): more requests now finish fast, and the tail is the GC (see
below), not the lock.

### Experiment 2: sync.RWMutex in real runs

A prototype backed `Mutex.with` with `sync.RWMutex.Lock` and `with_read`
with `RLock` when `barging(tk)` (real runs only), ignoring cancellation
and deadlock reports: the most a sync lock could gain. Same harness,
`xo-base` = the runtime lock (7bcb24d), `xo` = the prototype
(`XO_LOCKIMPL=sync`), load 3.88 at the start:

| Config | Req/s | p50 us | p99 us | p99.9 us |
|---|---|---|---|---|
| Go | 127,770 | 215 | 873 | 1429 |
| Xo, runtime lock | 63,180 | 408 | 1752 | 2493 |
| Xo, sync.RWMutex | 62,037 | 375 | 1786 | 2495 |
| Xo --arenas, runtime lock | 100,008 | 257 | 1139 | 1749 |
| Xo --arenas, sync.RWMutex | 101,267 | 233 | 1158 | 1676 |

No difference outside the spread (runtime lock 60.2k to 65.6k, sync
61.0k to 65.4k; arenas 99.0k to 104.9k against 96.0k to 104.2k). With
lock stats on, sync.RWMutex writers waited longer (mean 91 us against 8
us): new readers enter ahead of a writer that is still waiting for the
current readers to leave. Decision 0064: real runs keep the cancelable
runtime lock; the prototype was not kept.

### Next biggest cost: GC

The lock is no longer the gap. The Xo server is GC bound: 173 GCs per
10k requests against Go's 25, 15 percent GC CPU, a live heap of about 5
MB, so the default `GOGC=100` collects every few MB of allocation. One
uninstrumented run with `GOGC=800` (load 14, not comparable to the table
above) gave Xo 137k and Xo `--arenas` 136k req/s. Next experiments, in
order of likelihood:

1. Allocation profile of one request per route
   (`XO_CPUPROFILE` plus a `-memprofile` style `XO_MEMPROFILE` hook in
   `RunMain`, then `go tool pprof -sample_index=alloc_space`), and cut
   the top allocation sites in the router, `http.json`, and request
   decoding (Go allocates about a seventh as often per request).
2. A runtime GC target for servers: `debug.SetMemoryLimit` with a larger
   `GOGC` when `http.server` starts (or `XO_GOGC`), measured with
   `CONFIGS="go xo xo-arenas"` and peak RSS in the table; this changes
   memory use, so it needs an ADR.
3. The remaining hold gap (2 us for create and update) is the email scan
   of the Xo store; a solution with an email index (as Go's) would be a
   benchmark change in bench/xo, not a codegen one.

## 2026-10-08: allocations per request (non-arena build first)

Goal: cut the per request heap allocation of the Xo server toward Go's.
The primary measure is allocations and bytes per request per route,
which do not depend on host load (another agent ran load tests at the
same time; load average 7 to 25). Tool: `bench/perf/http/allocs.sh`
(new): `XO_MEMSTATS` / `MEMSTATS` counters of two fresh servers sent
500 and 5500 units of one route on one connection, differenced. Routes:
`get` (GET /users/1), `list` (GET /users, 64 users), `patch`,
`createdel` (POST + DELETE, mean of the two), `cycle` (the run.sh mix).
GCs/10k here are at one connection (a lower bound). Profiles:
`XO_MEMPROFILE` with `XO_MEMPROFILERATE=1`.

Before = main at b5381af (with the XO_MEMSTATS hook applied), after =
branch head. `xo` = `--no-arenas`, `xo-arenas` = the default since
decision 0066.

| Route | Go allocs | Go bytes | Xo before allocs | Xo before bytes | Xo after allocs | Xo after bytes | Xo arenas before | Xo arenas after (bytes) |
|---|---|---|---|---|---|---|---|---|
| get | 22.0 | 2326 | 71.0 | 8114 | 29.0 | 3596 | 40.0 | 23.0 (2950) |
| list | 26.1 | 5661 | 199.3 | 43238 | 35.3 | 18122 | 172.1 | 29.1 (8601) |
| patch | 44.1 | 3938 | 88.1 | 8978 | 43.1 | 4357 | 55.1 | 36.1 (3568) |
| createdel | 31.5 | 2803 | 62.1 | 6993 | 30.6 | 3453 | 42.6 | 26.1 (2927) |
| cycle | 30.3 | 2920 | 75.2 | 8803 | 33.2 | 4092 | 48.0 | 27.5 (3211) |

GCs per 10k requests at one connection, cycle: Go 9.4, Xo 30.4 -> 13.7,
Xo arenas 16.8 -> 10.1.

Sites fixed, in commit order (allocations per GET before -> after, no
arenas):

1. 8b65b93: measurement only (allocs.sh, `loadgen -route`, XO_MEMSTATS,
   XO_MEMPROFILE, Go MEMSTATS).
2. afddaae: a small `Map` was five allocations (table, index struct,
   index slots, log array, the array's header box for the atomic
   pointer); the first index (8 slots) and the first header now live in
   the table: two. 71 -> 56.
3. 8125716: router and server. Dispatch split the path once per route
   and built a parameter table for every route whose path matched,
   including the GET/PATCH/DELETE siblings of the one chosen; now one
   split into a stack array and one table. The server printed the
   request URL and parsed it again (two allocations): it reads
   `hr.URL`. `io.ReadAll` of an empty body allocated 512 bytes and a
   body was copied into a string: now nothing for an empty body and one
   exact buffer otherwise (unsafe.String, nothing else holds it). The
   request's Task, thread, and cancellation node are one `reqBlock`,
   and the handler no longer captures its locals in closures. 56 -> 38.
4. 2d00d4e: `Mutex` kept the held mutexes of a task in a map made per
   task (two allocations per request): now one inline entry in a
   `heldSet` (part of the reqBlock for requests). `Own` and
   `Mutex.with_read` called `MarkP(&v)`, which moved `v` to the heap for
   every type. 38 -> 35.
5. ace6bca: JSON encode. `JsonEnc` switched on `any(&v)`, a heap copy of
   every value, per field and per list element (the list route's 130
   allocations); now Str/Int/Bool are read through unsafe without an
   escape, list elements are encoded in place (`JsonEncP(&l.s[i])`),
   and generated codecs call `v.F.XoJsonEnc(tk, w)` directly for Option,
   List, Set, Map, Patch, and `derive Json` fields. `json.encode` builds
   in a pooled buffer and copies out once. List 176 -> 34.
6. ae1d416: maps of the server sized to their keys (`mapCap`; the
   default first log array is 4 entries, was 8), `Map_values_seq` and
   `Map_keys_seq` small enough to inline (the store's email scan cost
   two closures per create and update), `sort_by` sorts positions
   instead of (key, element) pairs. Cycle bytes 5158 -> 4092.

Under load (run.sh, `REPS=3 PORT=18190 CONFIGS="go xo-base xo
xo-arenas"`, `xo-base` = main b5381af default, i.e. arenas; load
average 7.6 at the start, 18 at the end, req/s indicative only):

| Config | Req/s | p50 us | p99 us | GC CPU % | GCs/10k req |
|---|---|---|---|---|---|
| Go | 126,835 | 217 | 872 | 4 | 24.6 |
| Xo arenas, before (main) | 101,413 | 253 | 1090 | 9 | 53.6 |
| Xo no arenas, after | 79,875 | 325 | 1300 | 14 | 113.3 |
| Xo arenas, after | 108,539 | 240 | 1025 | 8 | 44.4 |

The no arena build was 61k req/s and 173 GCs/10k at 7bcb24d (table
above, a different session). Under 32 workers the GC count falls less
than the bytes: one 5 s run with counters (`loadstats`) gave Go 30.8
allocations, 3.0 KB per request, and 1.31 MB allocated per GC cycle;
Xo no arenas 37.5 allocations, 5.3 KB, and only 0.54 MB per cycle. With
`GOMAXPROCS=4` the same Xo binary allocated 2.34 MB per cycle (Go 2.65)
and served about twice the requests. Both run at the 4 MB minimum heap
goal with a 1 MB live heap; the likely cause is per P span caching
counted against that small goal
(`knowledgebase/gotchas/small-heap-gc-per-p-spans.md`).

What is left, ranked by likely effect:

1. GC goal for servers: the 4 MB minimum goal on 12 Ps is now the main
   GC cost, not the bytes. Experiment: `debug.SetMemoryLimit(64 << 20)`
   with `debug.SetGCPercent(400)` when `http.server` starts (or
   `XO_GOGC`), measured with `CONFIGS="go xo xo-arenas"` run.sh plus peak
   RSS; changes memory use, needs an ADR.
2. Size class spread: the request path allocates map tables (144 B),
   log arrays, the reqBlock (about 560 B), JSON buffers. Folding the
   request header and parameter tables into the reqBlock (inline log
   arrays for up to 4 entries) would remove 4 allocations and two size
   classes per request.
3. `Mutex.with` copies the value to the heap (`f(tk, &v)` escapes):
   one allocation per write. A per mutex scratch slot is unsafe if a
   closure captures the `var` parameter past the call; codegen could
   prove it does not.
4. The top level `json.encode` copy of a struct (`new(T)` in JsonEnc)
   and the copy out of the pooled buffer: Http_json could encode
   generated types with a direct XoJsonEnc call and hand the pooled
   buffer to the server to write and release.
5. `list`: `values()` then `sort_by` copies the users twice (18 KB);
   codegen can sort the temporary in place when the receiver is dead.

## 2026-10-08: Fair comparison (equal GC settings for both languages)

Question: with the same GC settings for every server, does Xo beat,
match, or trail Go? Harness: `bench/perf/http/run.sh` unchanged, 32
workers, 2 s warmup, 10 s measured, 64 preloaded users, port 18290,
`GODEBUG=gctrace=1` for the GC count (GCs/10k = gctrace GCs per 10k
requests of the whole run, as above). Xo built from main at 74a71de with
a fresh `go build ./cmd/xo` (`.tools/bin/xo` was older than HEAD). Five
rounds; each round ran `REPS=1` run.sh once per setting with the three
servers in a rotated order (go, xo-arenas, xo / xo-arenas, xo, go / xo,
go, xo-arenas), so drift lands on all three. The GC environment
(`GOGC`, `GOMEMLIMIT`) was set for every server the same way. Before
each run.sh call the driver waited until the 1 minute load average was
under 4 (it always got there, after 105 to 180 s; the benchmark itself
pushes the load to about 12 on this 12 core laptop), and recorded
`uptime`: 1 minute load 3.38 to 3.98 at every start, 5 minute load
falling from 15.4 to 5.7 over the session (other agents were idle by
then). The GOMEMLIMIT set ran after the other two (17:27 to 17:40), not
interleaved with them. Medians of 5:

| Setting | Config | Req/s | Xo/Go | p50 us | p99 us | GCs/10k req | GC CPU % | Peak RSS MB |
|---|---|---|---|---|---|---|---|---|
| default GOGC | Go | 126,185 | 1.00 | 217 | 870 | 24.8 | 4 | 22.6 |
| default GOGC | Xo (arenas, default) | 107,512 | 0.85 | 242 | 1033 | 45.4 | 8 | 28.4 |
| default GOGC | Xo `--no-arenas` | 89,345 | 0.71 | 277 | 1161 | 88.4 | 11 | 28.0 |
| GOGC=800 | Go | 140,285 | 1.00 | 206 | 685 | 1.03 | 0 | 50.8 |
| GOGC=800 | Xo (arenas, default) | 140,165 | 1.00 | 206 | 705 | 1.25 | 0 | 55.5 |
| GOGC=800 | Xo `--no-arenas` | 139,802 | 1.00 | 206 | 716 | 1.58 | 0 | 55.8 |
| GOGC=off, GOMEMLIMIT=256MiB | Go | 135,494 | 1.00 | 215 | 656 | 0.14 | 0 | 243.6 |
| GOGC=off, GOMEMLIMIT=256MiB | Xo (arenas, default) | 136,721 | 1.01 | 212 | 673 | 0.16 | 0 | 245.0 |
| GOGC=off, GOMEMLIMIT=256MiB | Xo `--no-arenas` | 136,322 | 1.01 | 213 | 666 | 0.20 | 0 | 244.6 |

Run to run spread inside a setting was about 1 to 2 percent of req/s
(Go at GOGC=800: 139.5k to 141.4k; Xo arenas at default: 105.4k to
113.9k), so differences under 2 percent are noise.

Conclusion: under equal settings that take GC off the critical path
(GOGC=800, or GOGC=off with a 256 MiB limit), Xo matches Go: req/s
within 0.3 percent at GOGC=800 and 1 percent above Go with the memory
limit, both inside the noise, p50 equal, p99 within 3 percent. At the
default GOGC=100 Xo trails Go: 0.85x with arenas (the default) and 0.71x
without. The whole remaining gap is GC frequency: Xo still triggers
1.8x (arenas) and 3.6x (no arenas) as many collections per request at
the 4 MB minimum heap goal, from about 10 (arenas) to 40 (no arenas) percent more bytes per
request plus the per P span caching against a tiny goal
(`knowledgebase/gotchas/small-heap-gc-per-p-spans.md`). Request handling
CPU outside GC is at parity. Even with GC almost gone, Xo still runs 20
to 50 percent more GCs than Go (1.25 and 1.58 against 1.03 per 10k at
GOGC=800), so the allocation volume difference is real but no longer
costs throughput there. Next experiments, ranked:

1. The server GC goal from the earlier list (`debug.SetGCPercent(400)`
   plus `debug.SetMemoryLimit(64 << 20)` when `http.server` starts,
   ADR needed): this table says it would close the default setting gap
   for Xo users who never tune GOGC. Measure with
   `CONFIGS="go xo-arenas xo"` run.sh at default GOGC for Go and the
   new Xo default, plus peak RSS.
2. Cut bytes per request at default GOGC (the size class and copy items
   listed above, starting with folding header and parameter tables into
   the reqBlock), rerunning this exact table at default GOGC.
