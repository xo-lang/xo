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

Status: Proposed (Xo Lang 3's draft, reviewed by Xo Lang 2, who owns the
Windows native side; nothing is coded). Everything on the Windows poll
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
