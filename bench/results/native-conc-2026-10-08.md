# Native task runtime: concurrency benchmarks before and after decision 0077

Date: 2026-10-08 to 2026-10-09. Host: Apple M4 Pro (12 cores: 8
performance, 4 efficiency), macOS 27.0, Apple clang 21.0.0, Go 1.26.1.
Command: `bench/perf/conc/run.sh` (medians of 7 runs, interleaved: each
round runs every workload in every configuration once). Before: the
runtime at 180bad0 (decisions 0068 and 0069), built with that tree's `xo`.
After: the branch after the merge of main (40da705 plus the
documentation commit). Both tables were taken back to back with no other
agent running: `uptime` load averages 7.63 (before) and 6.01 (after), the
remainder of the test runs just before; CPU time and cycles are the
figures to trust, wall time is a median of short runs.

Configurations: `go` idiomatic Go (`bench/perf/conc/go`), `xo-go` the Xo
Go backend, `xo-llvm` and `xo-arm64` the native backends with one worker
per CPU (12), `xo-arm64-1` arm64 with `XO_THREADS=1`.

Workloads (`bench/perf/conc/xo`): `spawn` 200 rounds of 1000 tasks with
discarded handles adding to an Atomic; `fanout` 100 rounds of 1000 tasks
awaited in order; `pingpong` 100,000 round trips between main and a task
over two unbuffered channels; `mutex` 8 tasks x 25,000 `m.with`; `pmap`
20 rounds of 64 tasks over Collatz chunks.

## Ratios to the Go backend (12 workers)

| Workload | CPU before llvm / arm64 | CPU after llvm / arm64 | wall before llvm / arm64 | wall after llvm / arm64 |
|---|---|---|---|---|
| spawn | 6.68x / 6.08x | 0.77x / 0.73x | 3.45x / 2.99x | 0.42x / 0.40x |
| fanout | 5.09x / 5.36x | 0.49x / 0.53x | 2.47x / 2.45x | 0.34x / 0.36x |
| pingpong | 9.86x / 10.37x | 0.59x / 0.59x | 11.37x / 11.97x | 0.47x / 0.48x |
| mutex | 1.73x / 2.01x | 0.62x / 0.71x | 1.36x / 1.54x | 0.44x / 0.50x |
| pmap | 0.86x / 0.85x | 0.85x / 0.83x | 0.85x / 0.84x | 0.84x / 0.82x |

Target (native within 1.2x of the Go backend on every benchmark): met in
CPU time, cycles, and wall time, on both native backends.

## Before (180bad0)

Median wall ms

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 51.2 | 136.7 | 471.1 | 409.1 | 47.8 |
| fanout | 23.1 | 73.1 | 180.4 | 179.4 | 31.1 |
| pingpong | 21.7 | 57.6 | 654.5 | 688.8 | 711.2 |
| mutex | 23.7 | 116.1 | 157.3 | 178.4 | 14.8 |
| pmap | 55.8 | 73.4 | 62.1 | 61.4 | 512.6 |

Median CPU ms (user + sys)

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 168.5 | 339.3 | 2267.2 | 2063.2 | 68.5 |
| fanout | 60.4 | 174.0 | 885.8 | 932.5 | 43.2 |
| pingpong | 27.7 | 75.7 | 747.0 | 785.2 | 761.2 |
| mutex | 91.0 | 398.9 | 688.8 | 801.3 | 17.2 |
| pmap | 557.3 | 721.1 | 619.4 | 611.4 | 518.1 |

Median Mcycles

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 610.7 | 1292.8 | 8140.9 | 7345.8 | 255.2 |
| fanout | 228.8 | 638.6 | 3205.7 | 3301.1 | 154.3 |
| pingpong | 100.5 | 283.5 | 2451.1 | 2583.0 | 2479.6 |
| mutex | 333.5 | 1522.8 | 2462.2 | 2904.2 | 60.5 |
| pmap | 1955.4 | 2527.9 | 2163.7 | 2136.0 | 2090.4 |

## After

Median wall ms

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 51.1 | 134.5 | 55.9 | 54.4 | 31.5 |
| fanout | 24.0 | 73.1 | 25.2 | 26.0 | 20.7 |
| pingpong | 21.8 | 58.0 | 27.3 | 27.6 | 22.9 |
| mutex | 24.3 | 115.4 | 50.6 | 57.3 | 12.1 |
| pmap | 55.9 | 73.7 | 62.1 | 60.3 | 269.6 |

Median CPU ms (user + sys)

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 168.7 | 333.6 | 255.3 | 242.9 | 53.5 |
| fanout | 63.4 | 172.3 | 84.3 | 90.5 | 32.9 |
| pingpong | 27.8 | 76.9 | 45.1 | 45.7 | 32.2 |
| mutex | 98.3 | 396.5 | 244.8 | 282.5 | 15.3 |
| pmap | 557.6 | 726.6 | 618.8 | 606.0 | 525.7 |

Median Mcycles

| Workload | go | xo-go | xo-llvm | xo-arm64 | xo-arm64-1 |
|---|---|---|---|---|---|
| spawn | 628.8 | 1286.0 | 936.3 | 881.5 | 202.8 |
| fanout | 240.3 | 659.4 | 306.0 | 326.0 | 123.6 |
| pingpong | 100.5 | 287.8 | 161.6 | 165.5 | 122.8 |
| mutex | 363.8 | 1511.8 | 907.9 | 1060.0 | 55.6 |
| pmap | 1955.4 | 2545.4 | 2163.3 | 2126.7 | 2092.0 |

One worker did not get slower: spawn CPU 68.5 to 53.5 ms, fanout 43.2 to
32.9, pingpong 761 to 32 (main now runs the task itself while it waits),
pmap wall 513 to 270 ms (the main thread helps the one worker).

## What each step bought (arm64, 12 workers, CPU ms, 7 runs)

Taken during the work, at loads between 2 and 4 (indicative):

| Step | spawn | fanout | pingpong | mutex |
|---|---|---|---|---|
| before | 2020 | 910 | 760 | 770 |
| 9198503: Chase and Lev deques, Go's spinning rule, futex locks and parks, per node tree locks, atomic scope counts | 324 | 153 | 780 | 202 |
| + roots help while they wait, runnext with a grace | 432 | 213 | 44 | 306 |
| + Go style run queues (steal half), deadlock check at idle, padded hot words | 261 | 101 | 52 | 300 |

The roots step raised spawn CPU because every task was then stolen one
CAS at a time from the main thread's deque; stealing half of a ring per
CAS fixed it.

## Profiles and counters

`XO_RTSTATS=1` (llvm, 12 workers), spawn before: 47,879 condition variable
wake ups, 56.9 million spin rounds over all deques, 68,223 `sched_yield`
calls in spin locks; xctrace: 77% of cycles in `pool_pop` (idle workers
scanning deques under spin locks). After: 7,655 futex wake ups, 144,689
spin rounds, 68,973 steals (each takes half a ring), no yields. Ping pong
before: 99,976 thread waits and 99,994 thread wake ups (main on a
condition variable); after: none (the main thread runs the task's fiber).

What remains in spawn (xctrace, after): about a third of its cycles go
to the reference count of the `Atomic` handle that every spawned closure
captures (main retains it, every worker releases it: one contended cache
line, `xo_entry` and `xo.envdrop.main.fn1`), and to `count.add` itself;
the scheduler is no longer the top cost. The Go backend has no counts.

## Compute programs (no regression)

`bench/perf/llvm/run.sh` (best of 5 ms, Xo LLVM column), before / after:
fib 222.7 / 221.8, sieve 98.5 / 98.3, nbody 187.4 / 187.4, sort 214.8 /
214.5, interp 63.9 / 64.9, collatz 462.9 / 472.0 (rerun with 9 reps:
466.8 / 452.5), strings 33.6 / 33.2. Programs without tasks never touch
the scheduler.

## Correctness

- `go test ./internal/llvm ./internal/arm64` (goldens on both native
  backends, TestRuntimeTasks and TestRuntimeEventLoop under
  ThreadSanitizer with 1 and 3 workers and optimized, TestGoldenTasks
  under ThreadSanitizer with 1, 2, and 12 workers, TestDeadlock): pass.
- Every task golden (`conc_*`, `clock_timers`, `defer_blocks`,
  `http_server`, `net_echo`) 30 times each at 1, 2, 4, and 12 workers
  (llvm) and 15 times (arm64): same output, zero live allocations.
- The five benchmarks at a tenth of their size under ThreadSanitizer
  (TestGoldenTasks harness) at 1, 2, and 12 workers: clean. At full size
  ThreadSanitizer itself crashes with one or two workers, also before
  this change at larger sizes (`knowledgebase/gotchas/tsan-fiber-trace-crash.md`).

## Left

- Reference counts of handles captured by many tasks: a borrowed capture
  (no retain when the closure cannot outlive the scope) would remove the
  contended count; a compiler change (internal/ssa), not the runtime.
- Mutex `with` wakes every waiter on unlock (thundering herd); a FIFO
  hand off to one waiter would cut mutex CPU (now 0.62x to 0.71x).
- The LLVM backend emits load and store exclusive loops for shared counts
  (no LSE: the IR has no target CPU); arm64 uses `stadd`/`ldaddal`.
- Linux futex and x86_64 paths compile in principle but were not run.
