# 0301: Idle workers poll the network themselves

Status: Proposed (Xo Lang 2, 2026-10-21; for the owner's decision, no
implementation before it)

## Context

Xo Lang 3's syscall counts (strace -f -c, linux/arm64 in Docker, the CRUD
server of bench/perf/http at 32 connections) showed Xo at 2.9 futex calls
a request against Go's 0.34. Measured again with the runtime's own
counts (XO_RTSTATS; the futex and lock class counts added 2026-10-21):

- Under strace (5 s, 3,514 requests): 2.1 futex calls a request; worker
  wakes and idle sleeps 0.65 a request each, contended lock slow paths
  0.28.
- Without strace (Docker, 6 runs of 96k to 301k requests): wakes and
  idle sleeps 0.10 to 0.16 a request each, lock slow paths 0.03 to 0.12.
  strace slows every system call, so Xo's workers go idle between
  requests far more often than in a real run; most of the gap is the
  measurement.
- On the Mac (334k requests): futex waits 0.13 and wakes 0.14 a
  request; of those, the workers' idle sleeps and wakes are most (53,071
  sleeps of 59,593 waits). Server CPU a request: Xo 16 to 36 us, Go 43 us
  (Docker).

What is left is structural. One poller thread (poll.xo poll_main) waits
in epoll_wait or kevent and hands each ready descriptor's fiber to the
pool (pd_ready, park_signal, the inject queue, wakep). When the workers
are idle, every request costs a futex wake of a worker by the poller and,
when the worker runs out of work, a futex wait: the ulock pair of the Mac
profile, about 9% of the server's CPU. Go has no such handoff in the
common case: a thread with nothing to run blocks in netpoll itself and
runs the goroutine that becomes ready, waking another thread only when
it finds more than it can run.

## Decision (proposed)

Idle workers poll. Stages, each measured (futex calls a request from
XO_RTSTATS, server CPU a request, p99 at 32 connections) before the next:

1. A worker that finds no work and would sleep (go_idle) first tries to
   become the poller (a CAS on a pool word): if it wins, it waits in
   epoll_wait or kevent (with the timer heap's timeout) instead of on its
   futex word, and runs what it finds (pd_ready with TLS_W set puts the
   fibers on its own ring; it wakes another worker only for a second
   item, as Go's injectglist starts at most the idle Ps it needs). The
   poller thread stays as the fallback for the case where every worker
   is busy (it polls only when no worker holds the role), so readiness
   never waits for a worker to finish.
2. Waking a worker that is the poller: wakep, finding the idle worker in
   epoll_wait, writes to an eventfd (Linux) or EVFILT_USER (macOS)
   registered with the poll set, instead of its futex word (Go's
   netpollBreak).
3. Timers move with the role: the polling worker fires due timers before
   it waits; the poller thread does so only while no worker polls.
4. Windows (IOCP) keeps the poller thread; the change is for the epoll
   and kqueue layers.

Expected saving, from the numbers above: in the common case (the server
mostly idle between requests) the poller's wake and the worker's sleep
become one epoll_wait return on the worker that then runs the fiber: the
Mac's idle sleeps and wakes (0.12 to 0.16 a request each, of 0.13 to
0.20 futex waits and 0.14 to 0.22 wakes a request over four runs) go,
leaving the lock slow paths' futex calls (most slow paths end in the
spin; the HTTP idle marks went lock free on 2026-10-21, 40% fewer slow
paths). A request also skips the inject queue's lock and
the cache misses of moving the fiber to another core.

## Consequences

- poll.xo's loop becomes callable by a worker; pool.xo's go_idle grows a
  polling branch; wakep learns the break. ThreadSanitizer edges
  (gotchas/tsan-raw-epoll-no-edge.md) apply to the worker's wait as they
  do to the poller's.
- The deadlock check: a worker in epoll_wait is idle for dl_check, as one
  asleep on its futex (a registered descriptor can still wake it).
- Risk: a polling worker that runs a long fiber delays readiness for
  everyone else; the role is released before running anything, so the
  poller thread (or another idle worker) takes it.

## Alternatives

- Spinning longer before sleeping (XO_SPIN_ROUNDS): 16 to 1024 rounds did
  not reliably cut the futex calls in Docker, and decision 0077's
  addendum found 16 rounds cost 1.27x CPU in rwmutex for no p99 gain.
- Running the fiber on the poller thread itself: the poller has no
  worker record or scheduler stack; making it one is this decision's
  stage 1 by another name.
- Leaving it: the measured gap without strace is small (Xo's CPU a
  request is below Go's), so this is an optimization of about 9% of the
  server's CPU, not a correctness fix.
