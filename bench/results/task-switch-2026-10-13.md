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

- Windows (scripts/windows-batch.sh through the lead's queue, at
  0b7308e5): windows/amd64 153 pass of 153 goldens, windows/arm64 153
  of 153 built (not run: no host), LLVM runtime tests ok, `xo test`
  LLVM against Go 16 same.
- Linux arm64 (EC2, at 0b7308e5): internal/llvm and internal/arm64
  goldens and runtime tests with ThreadSanitizer (TestGoldenThreads
  included), the C path goldens, TestNoGCTests: pass; xo1 built by the
  Go `xo`; xo1 goldens with cc 153 pass of 153, with `--backend=llvm`
  153 of 153. The `--as=macho --ld=macho` step had no linux/arm64
  runtime pack (not a result), and archive_formats there stops at
  `svc #0`, which the ELF assembler does not take (rt.sys6, decision
  0206; not this change).
- Linux amd64 (EC2, at 94344471, before the fix of
  gotchas/llvm-thread-local-address-cse.md): TestGoldenThreads failed
  on clock_timers with a SEGV in xo_x_fiber_park (that bug; the fix is
  confirmed on the IR, the amd64 rerun is left to the lead's next Linux
  full tier); the C path goldens passed.

## Not done

Task spawn, done, await, and scope close; the inject queue, idle
workers, and steal rounds; fiber stacks; worker threads: what each
needs is in decision 0212's Stage 2 (done: the section below).

## Stage 2: the pool, locks, parks, stacks, threads, spawn and join

Decision 0212 stage 2: std/native/llrt/pool.xo (worker pool, the
locks' slow paths and futex waits, worker thread starts, fiber stacks,
parks, the cancellation tree, a spawn's link and queueing, a close's
help and join), called by xo_rt.c through cabi.xo; `XO_LLRT_C=sched`
still builds the C path. Host as above (Apple M-series, darwin/arm64,
about ten agents; instructions retired from `/usr/bin/time -l`). The
per operation pairs are stage 1's (spawn 200 and 600 rounds of 1000
tasks, pingpong 100k and 300k round trips, XO_THREADS=1, median of 9;
C path / Xo, Xo over C). Scratch scripts, through the heavy lock.

### C moved (against main at 4f792c55, which the branch merged)

| | main 4f792c55 | ac6797ee |
|---|---|---|
| std/llrt/xo_rt.c, all lines | 7,569 | 7,834 |
| active in a default build (`unifdef -UXO_SCHED_C`) | 7,088 | 6,565 (523 fewer) |
| compiled only with -DXO_SCHED_C (the C path) | 481 | 1,269 |
| compiled only without it (wrappers, declarations) | 114 | 190 |
| std/native/llrt/pool.xo | | 1,308 |

### Per operation, the Go driver

| commit | change | spawn arm64 C / Xo | ratio | spawn LLVM C / Xo | ratio | pingpong arm64 C / Xo | ratio | pingpong LLVM C / Xo | ratio |
|---|---|---|---|---|---|---|---|---|---|
| 73335916 | first cut | 2,139 / 2,258 | 1.056 | 1,931 / 1,900 | 0.984 | 3,724 / 4,362 | 1.171 | 3,654 / 3,741 | 1.024 |
| 8c55740f | the lock's first try at each site, the wait's flush and budget in the C | 2,137 / 2,245 | 1.050 | 1,928 / 1,896 | 0.983 | 3,758 / 4,195 | 1.116 | 3,599 / 3,737 | 1.039 |
| 8c55740f | the same, XO_SPIN_ROUNDS=0 | 2,128 / 2,236 | 1.051 | 1,908 / 1,882 | 0.986 | 2,922 / 3,031 | 1.037 | 2,687 / 2,847 | 1.059 |
| 68b1c7b7 | the C finds a wait's fiber and root record, libSystem's clock, the spinner's pauses unrolled | 2,140 / 2,244 | 1.049 | 1,928 / 1,898 | 0.984 | 3,682 / 3,787 | 1.028 | 3,465 / 3,469 | 1.001 |
| 24ec38e3 | a spawn queues in the spawner's ring the C found | 2,142 / 2,213 | 1.033 | 1,928 / 1,873 | 0.971 | 3,703 / 3,828 | 1.034 | 3,536 / 3,502 | 0.990 |
| ac6797ee | merged with main (final) | 2,136 / 2,217 | 1.038 | 1,925 / 1,872 | 0.973 | 3,723 / 3,804 | 1.022 | 3,531 / 3,537 | 1.002 |
| ac6797ee | the same, XO_SPIN_ROUNDS=0 | 2,135 / 2,215 | 1.038 | 1,923 / 1,870 | 0.972 | 3,512 / 3,728 | 1.062 | 3,382 / 3,374 | 0.998 |

The first cut missed the bar on arm64. With spinning off, arm64 ping
pong fell from 1.116 to 1.037: most of its difference was the spinning
worker, whose 32 pauses per round were a counted loop in Xo (128
instructions) where clang unrolls the C's to 32 yields
(gotchas/scheduler-instruction-counts-spin.md). The rest came from C
services called per operation (the root record and the fiber per wait,
the clock per wake) and a thread local read per spawn, now passed in
by the C or read from libSystem directly. At ac6797ee every pair is
within the bar with spinning on; with it off arm64 ping pong measured
1.062 (its C count moved from 2,922 to 3,512 between the two spin-off
rounds: ping pong per round trip still varies by 15 to 20 percent
between rounds, as stage 1 found).

### Per operation, xo1 (the self hosted driver, s3 of the fixed point at 8c55740f)

| spawn arm64 C / Xo | ratio | spawn LLVM C / Xo | ratio | pingpong arm64 C / Xo | ratio | pingpong LLVM C / Xo | ratio |
|---|---|---|---|---|---|---|---|
| 2,134 / 2,249 | 1.054 | 1,928 / 1,893 | 0.982 | 3,707 / 4,167 | 1.124 | 3,523 / 3,709 | 1.053 |

xo1 counted like the Go driver at that commit (both before the fixes
of 68b1c7b7 and 24ec38e3); xo1 was not measured again at ac6797ee (the
owner moved long runs off the Mac; the Go driver's ac6797ee pairs above
are the final figures).

### HTTP (server instructions per cycle unit, XO_THREADS=1, min of 5)

| commit | arm64 C / Xo | ratio | LLVM C / Xo | ratio |
|---|---|---|---|---|
| 73335916 | 647,120 / 654,300 | 1.011 | 594,058 / 591,651 | 0.996 |
| ac6797ee | 644,043 / 644,483 | 1.001 | 590,884 / 588,971 | 0.997 |

### Correctness

- Task goldens (20: clock_timers, every conc_*, fault_task*,
  select_arm_order, task_budget*, task_native) with the Go driver, arm64
  and LLVM, default and XO_LLRT_C=sched: 20 of 20 in all four at every
  stage 2 commit. Every golden (153) on both backends with the leak
  check at e1e30654: 153 pass each.
- internal/llvm TestRuntimeTasks, TestRuntimeEventLoop,
  TestRuntimeThreads, TestGoldenThreads (ThreadSanitizer), TestGoldenTasks,
  TestRuntimeAlloc: pass at 7f4ae2a3 and ac6797ee. cmd/xo TestNoGCTests
  (shutdown), TestRecordReplayCLI, TestRecordReplayStamp, TestXoReplay,
  TestSchedGoCallbackSeed, TestSchedSameSeedSameInterleaving (the
  deterministic scheduler is the Go backend's std/rt, unchanged): pass
  at 73335916 and ac6797ee. TestSelfhostWasm: pass at 73335916 and
  ac6797ee.
- Fixed point on the Mac at 8c55740f (seed: a cached xo1): s2 and s3
  assembly identical (436,743,854 bytes), binaries identical
  (87,305,136 bytes), s1 equals s2; s3's goldens 153 of 153 with arm64
  and with LLVM. The final commit's fixed point ran on Linux arm64 (below).
- `go test -short` over the repository: pass, but cmd/xo's run after the
  merge timed out at 600 s while its first native build rebuilt xo1
  (std changed); with XO_SELF_DRIVER=0 it passes in 51 s.
- Linux arm64 (EC2, r8g.8xlarge, at ac6797ee): internal/arm64 goldens
  and runtime tests pass; internal/llvm passes but for
  ThreadSanitizer races in net goldens (below); the C path goldens,
  TestNoGCTests and the scheduler tests pass; xo1 built by the Go `xo`,
  its goldens 153 of 153 with arm64 (cc) and 153 of 153 with LLVM;
  fixed point: s2 and s3 assembly identical (446,868,117 bytes),
  binaries identical (87,479,096 bytes), s1 equals s2. The XO_NOLIBC=1
  golden summary was not captured (the request printed only the
  failures' tails: fault goldens, a known limit of those builds).
- Linux amd64 (EC2, c7i.8xlarge, at ac6797ee): the C path goldens,
  TestNoGCTests and the scheduler tests pass; TestGoldenThreads reports a
  ThreadSanitizer race on a poll descriptor (pd_new's calloc on the
  dialing thread, the poller's lock of it). Main at 4f792c55 reports the
  same races (net_echo, http_wire, net_only) on the same host type: the
  poller's epoll calls are Xo system calls since decision 0206 stage 3,
  which ThreadSanitizer's epoll interceptors do not see. Not this
  change.
- Windows (EC2, scripts/windows-batch.sh, at ac6797ee): windows/amd64
  152 of 153 goldens pass, windows/arm64 153 of 153 built; the runtime
  tests fail only on net_only; `xo test` LLVM against Go 16 same.
  net_only crashes (SIGSEGV, exit 139 under Git Bash) after its
  redirect checks with the default path and with XO_LLRT_C=sched alike,
  and at main 4f792c55 too: not this change.

EC2 instances (launched and terminated by the lead's queue):
i-EXAMPLE54 (Linux arm64), i-EXAMPLE40 (Linux amd64),
i-EXAMPLE05 (Linux amd64, main), i-EXAMPLE50,
i-EXAMPLE11, i-EXAMPLE25 (Windows).

### Not done (decision 0212 stage 3)

Await and the wait lists, the task's record and end (allocation, the
entry call, drops, fault text), contexts and budgets, the deadlock check
and the test world's clock, Linux thread starts in libc builds, the
pool's setup: decision 0212's Stage 3 lists what each needs.
