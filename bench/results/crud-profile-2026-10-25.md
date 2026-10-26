# CRUD server under load: where the time goes (2026-10-25, Xo Lang 3)

Server: bench/perf/http/xo, native arm64, `xo build --release --profile-support`, run with
`XO_PROF=cpu`, loaded by bench/perf/http/loadgen (c=32, 2 s warmup, 12 s measured, loopback,
64 preloaded users), darwin/arm64, main at 40f67b3a2.

## Flat profile (46.9 s of CPU over 12 s, 149,912 req/s, 24.1 us CPU per request)

| function | flat | share |
|---|---|---|
| `_sendto` (hconn.send_at, via http1.respond and write) | 22.24 s | 47.4% |
| `read` (http1.fill, via Reader.read_one) | 15.91 s | 33.9% |
| `kevent` (poll.poll_round, cabi.np_poll) | 2.97 s | 6.3% |
| `_ulock_wait` (pool.go_idle, find_work) | 2.89 s | 6.2% |
| `_ulock_wake` (pool.futex_wake) | 2.15 s | 4.6% |
| http.dispatch, http.respond, the routes handler, Store.delete, mutexes | about 1 s each at most | about 3% together |

Two syscalls a request (one read, one sendto) are 81% of the CPU; parking is 10.8% (wait 6.2
plus wake 4.6); all Xo code of the request path is under 3%. No string work (slice, trim,
JSON) is on a hot path of this server.

## Twin comparison (bench/perf/http/run.sh, 2 reps, 8 s, medians)

| config | req/s | p50 us | p99 us | p99.9 us | CPU us/req | RSS MB |
|---|---|---|---|---|---|---|
| go (plain Go) | 121,726 | 219 | 922 | 1,521 | 35.46 | 22.2 |
| xo-arm64 | 150,962 | 191 | 626 | 981 | 23.28 | 18.7 |

Go ran 2,930 collections (370.8 ms of pauses, 4% GC CPU); Xo has none.

## Where the remaining time could be

The kernel's loopback send and receive (81%) are not Xo code; batching writes needs pipelined
requests (the bench is closed loop, one request in flight per connection). The 10.8% parking
(ulock wait and wake) is the scheduler's: a worker that finds no work parks and the next
readiness wakes one (pool.go_idle, find_work, futex_wake).
