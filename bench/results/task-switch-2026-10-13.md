# The task switch and scheduler core in Xo (2026-10-13)

Decision 0212 (decision 0195 prerequisite 6): the fiber switch and the
thread locals emitted by the compiler, and the scheduler core
(std/native/llrt/sched.xo: run queue ring, fiber frame, run, park, wake,
start, task claim and states) called by the C runtime through C ABI
exports. The C path is `XO_LLRT_C=sched` (-DXO_SCHED_C).

Host: Apple M-series Mac, darwin/arm64, about ten agents running (load
noted on every timing; the figures below are instructions retired from
`/usr/bin/time -l`, not wall time). Commits: the scheduler at 20cc4864
(after the merge 34a13cee of main's heap in Xo), measured with the Go
driver (XO_SELF_DRIVER=0) on both native backends.

## C moved

| | lines |
|---|---|
| std/llrt/xo_rt.c at the merge base cc567b6e | 7,349 |
| xo_rt.c now, all lines | 7,620 |
| compiled only with -DXO_SCHED_C (the C path: the switch assembly for four targets, the C ring, fiber code, task states, thread locals, the dispatcher of sched_c.xo) | 427 |
| compiled only without it (export declarations, wrappers, the TLS accessors) | 76 |
| active in a default build (7,620 - 427) | 7,193 (156 fewer) |
| std/native/llrt/sched.xo / sched_c.xo | 343 / 44 |

No assembly is left in xo_rt.c on the default path.

## The bar (decision 0180: Xo within 5 percent of the C)

### Per operation, XO_THREADS=1

Two builds per workload: bench/perf/conc spawn with 200 and 600 rounds
of 1000 tasks (per task: the difference over 400k tasks), pingpong with
100k and 300k round trips (per round trip: the difference over 200k).
Whole runs vary with the idle thread's spinning
(gotchas/scheduler-instruction-counts-spin.md); the difference cancels
setup. Instructions per operation, C path / Xo, and Xo over C.

Before the last two changes (rt.tlocal_addr and the fiber's hooks
word; both within the bar then):

| round | statistic | workload | arm64 C / Xo | ratio |
|---|---|---|---|---|
| 10:31 | min of 7 | pingpong, thieves' peek behind a call | 3,724 / 4,084 | 1.097 |
| 10:33 | min of 7 | pingpong, peek inline again | 3,755 / 3,933 | 1.047 |
| 10:37 | min of 7 | spawn | 2,114 / 2,200 | 1.041 |
| 10:37 | min of 7 | pingpong | 3,791 / 3,886 | 1.025 |

At 20cc4864:

| round | statistic | spawn arm64 C / Xo | ratio | spawn LLVM C / Xo | ratio |
|---|---|---|---|---|---|
| 10:40 | min of 7 | 2,134 / 2,148 | 1.007 | | |
| 10:44 | min of 7 | 2,135 / 2,162 | 1.013 | 1,912 / 1,873 | 0.980 |
| 10:45 | median of 9 | 2,144 / 2,152 | 1.004 | 1,884 / 1,868 | 0.992 |
| 10:45 | median of 9, XO_SPIN_ROUNDS=0 | 2,006 / 2,150 | 1.072 | 1,918 / 1,855 | 0.967 |
| 10:46 | median of 9, XO_SPIN_ROUNDS=0 | 2,121 / 2,151 | 1.014 | 1,886 / 1,858 | 0.985 |
| 11:56, at 94344471 (heap pointer in the table) | median of 9 | 2,125 / 2,195 | 1.033 | 1,924 / 1,867 | 0.970 |
| 11:57, at 94344471 | median of 9 | 2,123 / 2,154 | 1.015 | | |

| round | statistic | pingpong arm64 C / Xo | ratio | pingpong LLVM C / Xo | ratio |
|---|---|---|---|---|---|
| 10:40 | min of 7 | 3,305 / 3,242 | 0.981 | | |
| 10:44 | min of 7 | 2,206 / 3,341 | 1.515 | 3,169 / 3,000 | 0.947 |
| 10:45 | median of 9 | 3,503 / 3,133 | 0.894 | 2,521 / 2,738 | 1.086 |
| 10:45 | median of 9, XO_SPIN_ROUNDS=0 | 2,886 / 2,482 | 0.860 | 2,832 / 2,441 | 0.862 |
| 10:46 | median of 9, XO_SPIN_ROUNDS=0 | 3,243 / 2,867 | 0.884 | 3,345 / 3,341 | 0.999 |

Spawn holds the bar: arm64 1.004 to 1.033 in six rounds of seven (one
1.072 when the C path's run came out low), LLVM 0.967 to 0.992.
Pingpong's per round trip count moves by 15 to 30 percent between
rounds for either path (the 1.515 is a C run of 2,206, far below its
other rounds), so no single ratio is meaningful; the Xo path is below
the C in 7 of the 9 pairs.

### Whole runs, XO_THREADS=1, minimum of 15 (10:42, at 20cc4864)

| workload | arm64 C / Xo | ratio | LLVM C / Xo | ratio |
|---|---|---|---|---|
| spawn | 441.9M / 438.3M | 0.992 | 402.7M / 389.5M | 0.967 |
| fanout | 239.4M / 236.3M | 0.987 | 233.4M / 226.6M | 0.971 |
| pingpong | 312.1M / 225.0M | 0.721 | 327.6M / 306.3M | 0.935 |
| mutex | 113.4M / 102.5M | 0.904 | 94.3M / 93.4M | 0.991 |
| rwmutex | 366.6M / 361.1M | 0.985 | 329.5M / 311.7M | 0.946 |
| pmap | 3,880.3M / 3,880.9M | 1.000 | 3,886.6M / 3,886.0M | 1.000 |

With every CPU (same round) the counts spread further (pingpong arm64
299.6M against 381.7M, rwmutex LLVM 670.6M against 561.5M) in both
directions; they are not a measure of the code.

### HTTP (bench/perf/http, the users CRUD server)

Server instructions per `cycle` route unit (POST, GET, PATCH, GET,
DELETE, a list every eighth), one connection, XO_THREADS=1: a fresh
server per run, 5,500 units minus 500 units over 5,000, the minimum of
5 servers each.

| backend | C path | Xo | ratio |
|---|---|---|---|
| arm64 | 643,758 | 644,438 | 1.001 |
| LLVM | 592,084 | 591,935 | 1.000 |

(An earlier round of 3 servers: arm64 684,719 / 659,533, 0.963; LLVM
631,100 / 636,976, 1.009.)

## Correctness

- std/llrt/testdata tasks.c, evloop.c, rc_threads.c under
  ThreadSanitizer, one and three workers, both paths (TestRuntimeTasks,
  TestRuntimeEventLoop, TestRuntimeThreads): pass. Before the hook was
  excluded from instrumentation, tasks.c with three workers failed 4 of
  10 runs (ThreadSanitizer CHECK failed in its allocator; main's C
  runtime 0 of 10); after it, 0 of 10
  (gotchas/tsan-fiber-switch-in-callee.md).
- internal/llvm TestGoldenThreads (every golden on four threads under
  ThreadSanitizer, share probes), TestGoldenTasks, TestRuntimeAlloc;
  cmd/xo TestNoGCTests (with shutdown); TestRecordReplayCLI,
  TestRecordReplayStamp, TestXoReplay, TestSchedGoCallbackSeed,
  TestSchedSameSeedSameInterleaving: pass. The deterministic scheduler
  (--seed, --explore, replay) is the Go backend's (std/rt), which this
  change does not touch; native builds refuse those flags.
- The selfhost differentials on 17 task and fault programs
  (TestSelfhostSSA, TestSelfhostArm64, TestSelfhostLLVM with
  XO_SELFHOST_DIRS, at 525dfafe): 17 of 17 runs byte identical, 2,804
  arm64 and 2,821 LLVM functions identical.
- selfhost/macho against clang on the empty program and on http_server,
  darwin and linux objects: byte identical (after adding `prfm`, which
  main's heap in Xo had made fail every `--as=macho` build, and `sxtw`).
- xo1 on darwin/arm64 builds and runs conc_stress, conc_select,
  conc_scopes, conc_channels, conc_mutex, conc_unwind, fault_tasks,
  fault_task_exit, clock_timers, http_client, http_tls, http_server with
  `--as=macho --ld=macho` (a local runtime pack), with cc, and with
  `--backend=llvm`: output as want.txt (http_server differs only in its
  normalized time stamp).
- The Go driver, eight task goldens, arm64 and LLVM, default and
  XO_LLRT_C=sched: 32 of 32 as want.txt; the C path binary holds
  `xo_fiber_switch`, the default one `xo_stack_switch` and
  `xo_fiber_entry`.
- TestSelfhostWasm: 133 pass, 20 unsupported, 0 fail of 153.
- `go test -short` over the repository: pass.

Linux (EC2 arm64 and amd64) and Windows runs: below when they report.

## Not done

Task spawn, done, await, and scope close; the inject queue, idle
workers, and steal rounds; fiber stacks; worker threads: what each
needs is in decision 0212's Stage 2.
