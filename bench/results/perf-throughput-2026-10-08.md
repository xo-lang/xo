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
