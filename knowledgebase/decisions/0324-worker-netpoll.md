# 0324: Idle workers poll the network themselves (Go's netpoller model)

Status: Accepted (owner, 2026-10-21): Linux (epoll) and macOS (kqueue) first, Windows' completion ports a later step of their own

## Context

The hybrid CRUD server (bench/perf/http, Docker linux/arm64, 1024
connections, the load generator on other cores; Xo Lang 3's runs of
2026-10-21) serves fewer requests on 6 cores than on 3:

| per request | 1 core | 3 cores | 6 cores |
|---|---|---|---|
| requests a second | 20.7k | 68.1k | 62.0k |
| worker wakes (wake) | 0.048 | 0.033 | 0.216 |
| idle sleeps | 0.048 | 0.033 | 0.216 |
| inject queue adds | 0.279 | 0.187 | 0.511 |
| steals | 0.064 | 0.062 | 0.178 |
| futex waits / wakes | 0.083 / 0.085 | 0.064 / 0.070 | 0.370 / 0.406 |
| fiber parks | 1.94 | 1.93 | 1.94 |

The work per request is constant (fiber parks); what grows with cores is
coordination. Capping the pool (XO_THREADS=3 on 6 free cores) does not
recover it: the counters follow the free cores the threads land on, not
the pool size.

The path of every network event: the poller thread (std/native/llrt
poll.xo poll_main, a thread of its own) returns from epoll_wait;
pd_ready resumes the parked fiber; the resume comes from a thread with no
worker record, so it goes to the inject queue (its lock) and wakep
futex-wakes an idle worker unless one spins; the worker runs the fiber,
which parks within the request, and the worker, out of work, parks
(futex_wait). That is about one inject and wakep per request (poll_wake
933,196 for 933,290 requests at 6 cores), and with more free cores more
of them land on a parked worker on another core.

Step 1 (spinning before parking, XO_SPIN_ROUNDS; 64 rounds were
measured but are not the default: on the Mac they cost 7% more CPU a
request on bench/perf http, 2026-10-21, so the default stays 0; measured in the same runs: 6 cores 55.8k to 73.7k requests a
second, 13% less CPU a request) shortens the idle gaps but leaves the
futex calls at about 0.38 a request and 6 cores still below 3.

## Decision

Go's netpoller model: an idle worker waits in the poller itself.

1. A worker that finds no work (find_work, after its spin) takes the
   pool's poll token (one waiter in epoll_wait or kevent at a time) and
   blocks in epoll_wait itself, with the next timer's deadline as its
   timeout, instead of futex_wait. Without the token it parks as now.
2. The fibers its events make ready go to its own ring, the first to its
   next slot, and it runs them; it wakes one other worker only when more
   than one became runnable and no worker spins (Go's injectglist).
3. A worker that runs out of work with the token free, before parking,
   polls with no timeout once (Go's findRunnable), and runs what it gets.
4. Work for an idle pool while the token is held (a spawn, a channel
   send, a timer) breaks the waiter's epoll_wait through the existing
   self pipe (Go's netpollBreak), only when no worker spins.
5. The poller thread stays for what has no worker: signals, the timers
   while no worker polls, and the time before the pool starts. Windows
   (IOCP) and the test world keep their paths until measured.

## Consequences

- Expected at 6 cores: inject adds from 0.51 to under 0.1 a request
  (events land on the polling worker's ring), worker wakes and idle
  sleeps from 0.216 to under 0.05, futex calls from about 0.4 to about
  0.1; requests a second at or above 3 cores', where the shared count
  traffic (drop Str, the map walk) becomes the limit.
- The runtime's poll loop moves into the scheduler: pool.xo and poll.xo
  change together; the timer heap is read by the polling worker under
  the poll lock it takes today.
- A risk: a worker blocked in epoll_wait while runnable work waits on
  another worker's ring (a missed break). The break on every push to an
  idle pool, and the monitor's tick (decision 0286), bound it.
- Measured before it lands: the pinned 1, 3, 6 core runs with
  XO_RTSTATS (two interleaved rounds), bench/perf http, conc, and the
  async preemption tests; TSan's shared map test under --race.

## Implementation (2026-10-21, darwin and Linux)

- poll.xo: the token (EV_TOK: free, a worker's poll, a worker waiting,
  the poller thread), poll_round (one round of the old loop), np_poll
  and np_break (called through cabi.xo exports: poll.xo imports
  pool.xo). pool.xo: find_work polls without waiting after the inject
  queue; go_idle waits in the poller (np_block, counted by DLK_NPBLK,
  which the deadlock check counts as idle); wakep breaks the wait when
  every other worker is busy. XO_NETPOLL=0 keeps the poller thread path.
- After a poll that readied more than one fiber: one wakep, not a wake
  for each idle worker. Waking one per fiber cut inject to 0.28 a
  request but raised wakes and futex calls 12 to 19% over the old path.
- The poller thread ticks from 0.5 to 10 ms while workers poll (doubling
  while it finds nothing), sleeps while a worker waits in the poller, and
  waits in the poller itself after 10 empty ticks at 10 ms.
- A worker's poll never waits after it fired a timer: the timer's fiber
  is on the worker's own ring (gotchas/worker-poll-fires-timer-then-blocks.md).
- Measured (bench/perf http on the Mac, 256 connections, 8 runs a mode
  in two ABBA sets): inject 0.71 to 0.0003 a request, worker wakes -28
  to -34%, futex calls -25 to -30%, CPU a request -7 to -11%; requests a
  second -6.5% and +33% in the two sets, within the loaded Mac's noise.
  The connection resets the load generator sometimes reports (a 256
  connection burst against macOS's listen queue of 128) occur in both
  modes.
- Roots wait in the poller too (root_idle through np_block with main's
  fiber, so a program that never spawns waits there): main's end breaks
  that wait (main_ended calls np_break; np_block rereads main's end after
  its count went up). Golden rt_poll_root (main's own sleeps, then main
  ending while the root may wait in the poller): 0.51 and 0.32 s to 0.20
  and 0.14 s on the Mac.
- np_break also writes the self pipe while the token is free: a worker
  counted in DLK_NPBLK that has looked for work but not yet taken the
  token would otherwise wait with the push unseen (found by reading; a
  pusher with no worker record could leave it there until the next
  event). With a worker or the poller thread holding the token, that
  worker waits on its futex instead, where go_idle's check sees the push.
- Linux (Docker linux/arm64 on the Mac, 12 CPUs; the CRUD server
  bench/perf/http/xo pinned with taskset, loadgen on 6 other CPUs, 1024
  connections, 4 interleaved runs a cell, with the worker count from the
  affinity mask, gotchas/worker-count-ignored-cpu-affinity.md): 1 core
  +35% requests a second (4 of 4 pairs), CPU a request 25.0 to 16.9 us,
  futex waits 0.054 to 0.021 a request; 3 and 6 cores -4% and -2% (2 of
  4 pairs lower each, within noise), CPU a request the same, futex calls
  +25 to 28% and steals +60 to 83% a request (the fibers a poll readies
  land on one worker's ring and the others steal them). Inject 0.07 to
  0.23 a request to under 0.006 everywhere. A quiet server: 0 CPU ticks
  in 20 s both ways. At 3 and 6 cores the server used about half of its
  CPUs; a load generator on 9 CPUs gave the same (3 cores: about 50k
  both ways, netpoll ahead in 3 of 4 pairs, CPU a request -9%), so the
  client is not the limit: the server idles about half its CPU at 3 and 6
  cores, a serialization inside it (its store's lock: lock_slow 0.05 to
  0.08 a request) rather than the scheduler's polling.
- Before the affinity fix (12 workers on every pinning) the same table
  read 1 core +8.5%, 3 cores -14%, 6 cores even.

## Alternatives

- More spinning only (step 1 at higher counts): the futex calls stay.
- A per-core poller thread: more threads to wake, the same handoff.
- Waking the worker that last ran the fiber (affinity): helps the cache,
  not the wake count; can follow this.

## Proposed stage 2026-10-22: Windows workers poll the completion port

Status: Implemented 2026-10-22 (poll.xo; Xo Lang 3's draft, reviewed by Xo Lang 2,
who owns the Windows native side). First Windows run: CI 39878309838 on
the xo-windows runner (scripts/ci/windows-netpoll.sh): 144 runs ok, 0 failed
(12 goldens, worker polling on and XO_NETPOLL=0, XO_THREADS unset, 4 and 1,
two rounds); correctness and counters only, no timing. Mac dry run before it:
windows/amd64 cross builds, runtime layout tests, arm64 and LLVM concurrency
and http goldens.

Note (Xo Lang 2's review, 2026-10-22): while the poller thread waits in
GetQueuedCompletionStatusEx holding TOK_POLLER (its quiet state, wait -1),
a worker cannot take the token, and np_break posts only when the token is
TOK_BLOCKED or TOK_FREE; as on Linux. On Windows a post then wakes the
poller thread, not a worker. Everything on the Windows poll
path is untested on Windows (cross links only), so the first run is the
proof.

Today (poll.xo at 3a256049d): waiters arm a one shot AFD poll
(`pd_arm`, under the descriptor lock, from any thread) and only the
poller thread dequeues the port (`poll_main_win`,
GetQueuedCompletionStatusEx), calls `afd_done` per completion (the
readiness goes to the inject queue), fires timers, frees the dead list
and counts signals. `np_wanted()` is false there, so every wake takes
the inject queue and a woken worker, the cost this decision removed on
Linux and macOS.

Stage:
1. `np_wanted()` is true on Windows when the AFD device opened
   (`afd() != 0`; the WSAPoll fallback keeps the poller thread), not in
   the test world, and XO_NETPOLL not 0.
2. `wait_events` gets a Windows arm: GetQueuedCompletionStatusEx on the
   port for at most NEVENTS entries. `got` is zeroed before the call (a
   FALSE return with WAIT_TIMEOUT is no events).
3. `poll_round` decodes an OVERLAPPED_ENTRY (32 bytes, lpOverlapped at
   +8): 0 is the break (the PostQueuedCompletionStatus `poke` already
   posts, so np_break is unchanged), else `afd_done`. It needs no new
   flag: pool.xo `np_poll` sets W_NONEXT to 2 on the calling worker and
   `park_signal` reads the current worker, so a worker's completions land
   on its ring and the poller thread's still go to inject.
4. The token, np_spread (one wakep), and the poller thread's fallback
   (EV_PSLEEP, the tick) are as on Linux; the poller thread stays out of
   the port while a worker holds the token. A port takes many dequeuers
   (`afd_done` is thread agnostic), so the token is for one timer owner,
   break accounting and keeping wakes off the LIFO spread, not safety.
5. **Concurrency value (blocker found in review).** `ev_init` creates the
   port with NumberOfConcurrentThreads 1 (CreateIoCompletionPort at
   poll.xo ~1593). A thread that dequeued counts as running until it
   blocks in a wait again, and a worker running fibers does not, so the
   next dequeuer would not be released while packets wait. It must be 0
   (the processor count) with this stage.
6. **Timeouts.** The port's millisecond timeout follows the system timer
   (15.6 ms by default), so a worker's wait of 0 < wait < 1 ms passes 0
   and the worker loops; no timed GQCSEx carries a sub millisecond wait.
   The run-next grace wait (np_block when any_work is 2) and
   go_idle's futex_wait_us have the same problem on Windows and are fixed
   there by spinning or SwitchToThread for the grace (Xo Lang 2).

Not a risk (reviewed): an AFD request is not cancelled when the thread
that issued it exits (the handle is associated with the port in
`afd_open`, and arming already happens on workers).

Verification: windows/amd64 cross build of bench/perf/http and conc, the
runtime layout and static word tests, Wine single threaded smoke only
(owner, 2026-10-21), then one correctness run on the xo-windows runner
through the lead with XO_RTSTATS (inject, wake_push, idle_npblock,
poll_return, idle_sleep). No timing claim comes from it (owner,
2026-10-22: timing A/Bs run on the Mac in quiet windows only); expected
direction as on Linux, inject adds near 0.

## Measured on the linux/arm64 host (2026-10-30, Xo lang 5)

The owner's "measure first" on decision 0301, which this decision built
(bench/results/worker-netpoll-linux-2026-10-30.md): the CRUD server of
bench/perf/http, the server on 6 CPUs and the load generator on 6 others,
20 interleaved rounds, no CPU steal. Medians:

| conc | server | req/s | p50 us | p99 us | CPU us/req | futex calls/req |
|---|---|---|---|---|---|---|
| 32 | Go 1.26.1 | 182,614 | 127 | 891 | 18.62 | |
| 32 | worker polling | 219,486 | 120 | 703 | 13.62 | 0.31 |
| 32 | XO_NETPOLL=0 | 186,717 | 155 | 686 | 12.87 | 0.35 |
| 256 | Go 1.26.1 | 196,235 | 584 | 5,062 | 20.66 | |
| 256 | worker polling | 224,798 | 1,174 | 3,768 | 16.01 | 0.45 |
| 256 | XO_NETPOLL=0 | 226,512 | 1,084 | 3,538 | 15.00 | 0.31 |

- Worker polling: +17.6% requests a second at 32 connections in every
  round, -0.8% at 256 in every round (its readied fibers stolen: steals
  0.34 against 0.17 a request), for 6 to 7% more CPU a request. The
  default stays on.
- Against Go: 14 to 20% more requests a second, p99 20 to 26% lower, CPU
  a request 23 to 27% lower, at both levels in every round.

## 256 connections: three spreads tried (2026-11-01, Xo Lang 4)

The gap above at 256 connections, from Xo Lang 5's next ideas: a poll's
readied fibers land on the polling worker's ring and others steal them.
Three switches behind an environment variable on a branch that was not
merged (l4-np): a keeps at most K of a poll's fibers on the ring and injects
the rest; b keeps K and hands the rest, one each, to the run-next slot of
workers asleep on their futex, waking only those; c makes a worker's poll
take at most K events. bench/perf/http on the linux/arm64 host, OFF_EXCLUSIVE,
10 s runs, order rotating, medians per mode. The poller thread and a and c ran
10 to 11 rounds (stopped early once settled), b 3 rounds in its own run with
its own baseline. These are host http bench figures, not `--ab`. Counters
per request divide the whole run's counts, warmup included, by all its
requests.

| connections | mode | req/s | p99 us | futex_wait/req | steal/req | inject/req |
|---:|---|---:|---:|---:|---:|---:|
| 256 | worker polling | 214,960 | 3,955 | 0.195 | 0.333 | 0.000 |
| 256 | XO_NETPOLL=0 | 216,969 | 3,665 | 0.137 | 0.177 | 0.388 |
| 256 | a1 | 214,780 | 3,955 | 0.223 | 0.156 | 0.643 |
| 256 | a4 | 214,494 | 3,972 | 0.219 | 0.220 | 0.405 |
| 256 | c4 | 214,847 | 3,941 | 0.199 | 0.329 | 0.000 |
| 32 | worker polling | 209,894 | 740 | 0.139 | 0.283 | 0.000 |
| 32 | XO_NETPOLL=0 | 178,238 | 718 | 0.165 | 0.138 | 0.550 |
| 32 | a1 | 212,811 | 738 | 0.144 | 0.118 | 0.675 |
| 32 | a4 | 208,879 | 741 | 0.143 | 0.223 | 0.288 |
| 32 | c4 | 210,165 | 732 | 0.139 | 0.271 | 0.000 |

Experiment b's run (3 rounds):

| connections | mode | req/s | p99 us | CPU us/req | futex_wait/req | steal/req |
|---:|---|---:|---:|---:|---:|---:|
| 256 | worker polling | 215,612 | 3,941 | 16.7 | 0.194 | 0.332 |
| 256 | b1 | 209,668 | 4,153 | 19.1 | 0.245 | 0.412 |
| 256 | b4 | 210,628 | 4,127 | 18.5 | 0.227 | 0.384 |
| 32 | worker polling | 209,747 | 742 | 14.3 | 0.139 | 0.284 |
| 32 | b1 | 195,454 | 769 | 19.4 | 0.237 | 0.471 |
| 32 | b4 | 203,191 | 758 | 17.2 | 0.194 | 0.397 |

- None closes the gap, and none ships. The gap itself reproduced: worker
  polling -0.9% req/s against the poller thread at 256, p99 +8%, futex_wait
  +42%, and +18% req/s at 32.
- Steals are not the cost. a1 halves them and req/s does not move, because
  the work goes to the inject queue instead and futex calls rise.
- Waking sleepers is worse. b's hand-off did run (156,000 to 310,000 hands a
  run at b1), and it costs 2.3% to 6.8% req/s and 11% to 36% more CPU a
  request, with more futex calls and more steals; woken workers finishing
  and sleeping again would fit that, but it was not traced. b1's first round at 32 connections was 169,979, an outlier
  the median leaves out; the b4 and b1 differences hold in all 3 rounds.
- c4 changes nothing measurable.
- a1 at 32 connections is the one gain: +1.4%, quartiles not overlapping,
  and level at 256. Not enough to change the default without an `--ab` and
  a look at more connection counts.

## Where the extra futex waits go, and lock spin rounds (2026-11-01, Xo Lang 4)

Traced at 256 connections (futex_wait a request, by caller): 0.203 with worker
polling, 0.141 with `XO_NETPOLL=0`. Of the +0.062, +0.050 are waits in
`lock_slow`, not idle sleeps as predicted; `lock_mutex` a request is 0.170
against 0.133, which by inference is the bench's store `Mutex`. Steals double,
and earlier experiments here showed they are not the cost.

Decision (owner, 2026-11-01): `lock_slow` spins 1600 rounds on Linux before it
sleeps, in place of 100; other systems stay at 100 because they are not
measured.

- Against 100 on linux/arm64, 10 rounds: at 256 connections req/s +3.1%, p99
  -1.5%, p99.9 +2.9%, ranges overlapping; at 32, req/s +1.5%, p99 and p99.9
  level. `--ab`: self on amd64 wall 0.981, CPU 0.990; httpstreamsrv on arm64
  wall 0.977, CPU 0.976; self on arm64, httpstreamsrv on amd64, and alloc within
  noise. Oversubscribed at 2x CPUs, CPU does not rise. All of it is in
  bench/results/lock-spin-2026-11-01.md.
- It removes sleeps, not contention: `lock_slow` entries stay flat. It does not
  close the gap to the poller thread in CPU a request at 256.
- 3200 read higher in a sweep on an older main (c256: 100 222,833, 800 226,649,
  1600 229,791, 3200 233,713, 3 rounds); 1600 is the value with `--ab` data, not a proven best.
- Alternatives: leave 100 (forgoes the gain); a lock-specific fix to the store's
  `Mutex` (the bench's, not the runtime's: helps one program); adaptive spinning
  as glibc's (not tried).

## a1: a poll's batch past its first fiber is injected, Linux only (owner, 2026-11-02, Xo Lang 4)

A worker that polls kept every fiber its poll readied on its own ring
(W_NONEXT 2), so other workers stole them one at a time. a1 keeps the
first (`np_keep()`, 1) and injects the rest, where any worker takes them,
and np_spread wakes a worker for the one left. A poll's batch is now
marked W_NONEXT 3, apart from a mutex's reader grants (2), which stay on
the granting worker as decision 0077's addendum says.

The first experiment changed both batches. Measured apart (bench/perf/http
with a warm-up, 5 rounds, medians; and `xo bench --ab`), the two halves
help different cases:

| change | linux/arm64 c32 | linux/amd64 | Windows | --ab httpstreamsrv (arm64) |
|---|---|---|---|---|
| P: poll batches only | req/s -0.8%, CPU a request -5.9%; c1024 +2.7% | c32 -3.4%, c1024 +5.4% | c32 +9% (pinned), c1024 worse with load generator errors | CPU 0.886, RSS 0.94 |
| R: reader grants only | req/s +4.1%, CPU -4.3% | +8 to +11% at every count | c1024 -17% with errors | CPU 1.006 (amd64 RSS 1.117) |

The owner chose P on Linux only (2026-11-02). R stays parked until a
benchmark with real reader contention exists: `conc`'s rwmutex runs
about 6 ms with CPU equal to wall, so it shows no parallel readers, and
both halves read level there (1.006, 1.009, 15 rounds). darwin and
Windows keep every readied fiber (np_keep() 0), so their behavior is
unchanged.

**R on a contended benchmark (2026-11-02, Xo Lang 4).** bench/perf/rwread
(256 tasks on one `Mutex`, `with_read` with writes swept, `XO_THREADS`
swept) on the first linux/arm64 host (a Linux VM on an M4 Pro), R against
main, 5 rounds: at 100 writes in 1,000, reads a millisecond 0.73 to 0.75
at 2 to 12 threads (ranges apart), CPU 1.15 to 1.28; at 10 writes wall
0.93 to 0.97 but CPU 1.12 to 1.15 from 4 threads; level at 1 write and at
one thread; RSS level. R stays parked
(bench/results/a1-poll-batch-inject-2026-11-02.md).

Gate (a default change): on linux/arm64, the full internal/arm64
TestGoldenRun and internal/stdtest suites and the fixed point (xo built
with P rebuilds itself to the same bytes) pass on linux/arm64 (second
host, 2026-11-02, d424e0c5b: arm64 134 s, stdtest 1312 s, bootstrap s1
72 s, s2 126 s, s3 119 s); a confirming `--ab` of the landed branch
against its parent (a144a3115 against 7e5f1c0d1, first linux/arm64 host, 3 rounds, host load 8 on both sides): bench/perf/httpstreamsrv CPU 0.893 (0.883 to 0.913, better), wall 0.992 and RSS 0.96 within noise.

**Reverted (2026-11-02, Xo Lang 4).** Landed as 25d23d18e, P cost
bench/perf/jitter's /alloc on the first linux/arm64 host: req/s 132,426 against 100,127 at 32 connections (0.756, ranges apart) and 134,645 against 120,487 at 256 (0.895), CPU a request 1.037 and 1.064; p99 at 256 2.9 ms against 6.2 ms (one job, l4pts12: a144a3115 against 25d23d18e, 3 rounds, quiet host). The
owner's decision was to land P after the 0.36 /alloc regressions were
fixed; it went in before, and the release's /alloc row comes first. P
comes back with an /alloc row of its own beside the http ones above.
