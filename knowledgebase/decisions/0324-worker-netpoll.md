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

Step 1 (spinning before parking, 64 rounds by default with more than
one worker; measured in the same runs: 6 cores 55.8k to 73.7k requests a
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

## Alternatives

- More spinning only (step 1 at higher counts): the futex calls stay.
- A per-core poller thread: more threads to wake, the same handoff.
- Waking the worker that last ran the fiber (affinity): helps the cache,
  not the wake count; can follow this.
