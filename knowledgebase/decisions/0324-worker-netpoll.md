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
- Not yet: roots (root_idle) do not wait in the poller; Linux pinned 1,
  3 and 6 core runs (Docker) follow.

## Alternatives

- More spinning only (step 1 at higher counts): the futex calls stay.
- A per-core poller thread: more threads to wake, the same handoff.
- Waking the worker that last ran the fiber (affinity): helps the cache,
  not the wake count; can follow this.
