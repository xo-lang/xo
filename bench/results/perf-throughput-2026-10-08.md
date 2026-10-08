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
