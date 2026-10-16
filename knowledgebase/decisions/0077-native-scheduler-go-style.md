# 0077: The native task scheduler follows Go's: per thread run queues, spinning workers, roots that help, deadlock checked at idle

Status: Accepted (2026-10-09). Refines decision 0068 points 1 and 7 and
decision 0069 point 1 (the scheduler under the fibers; their semantics are
unchanged). Implemented in `std/llrt/xo_rt.c` ("The worker pool", parks,
the cancellation tree, scopes). Evidence:
`bench/results/native-conc-2026-10-08.md`, benchmarks
`bench/perf/conc/run.sh`.

## Context
A spawn heavy program ran 0.33 to 0.48 s natively against 0.15 to 0.24 s
on the Go backend, but 0.06 s with one worker. Profiles
(`bench/perf/conc/profile.py`, `XO_RTSTATS=1`) showed the cost was cross
thread coordination, not task work: idle workers scanned every deque under
spin locks (77% of cycles in `pool_pop`), every task took four global
locks (the cancellation tree, the deadlock counters, the fiber cache, the
deque) and its scope's mutex twice, a spawn could signal a condition
variable, and the main thread was not a worker, so channel ping pong with
a task paid two thread wake ups per round trip (12x the Go backend).

## Decision
1. **Run queues are Go's runq.** Every worker thread and every root thread
   (one that runs Xo code outside tasks: the program's main, C tests) that
   spawns has a 256 slot ring: only the owner adds (no lock), the owner
   takes from the head with a CAS, a thief takes half of the ring with one
   CAS, a full ring moves half to the inject queue (a locked queue for
   threads without a ring, the poller) that workers take a share of.
2. **runnext.** A fiber woken by a thread goes to that thread's `next`
   slot and runs before its ring; thieves take it only after 50 us
   (`XO_NEXT_GRACE_NS`), so a context that wakes another and then waits
   hands over on the same thread with no wake up.
3. **Roots help while they wait.** A root thread that waits runs queued
   work (its own ring and next first, then the inject queue and one steal
   round) on its stack as a scheduler of fibers, and sleeps only when it
   finds none. The main thread stays a plain thread (not a fiber), so
   single threaded programs and the C tests are unchanged.
4. **Waking follows Go's spinning rule.** A push wakes an idle worker only
   when none is spinning; the woken worker spins a bounded number of steal
   rounds (16, `XO_SPIN_ROUNDS` tunes); a spinning worker that finds work
   wakes another only if it was the last spinner; at most half the busy
   workers spin. Idle workers sleep on a futex word of their own. Workers
   never exit (the one per CPU limit holds since 0069).
5. **Locks and parks are futex words.** `xo_lock` (spin 100 times, then
   sleep on the word) guards tasks, scopes, channels, and Mutex: zero is
   unlocked, so making a task needs no initialization. A park is a lock
   and a futex word. Darwin uses `__ulock_wait`/`__ulock_wake` (libc++'s
   std::atomic::wait), Linux `futex`.
6. **The cancellation tree has a lock per node**, taken parent first; a
   task's node stays linked until its scope closes (the scope unlinks its
   node of tasks before it frees them).
7. **A scope's counts are one atomic word** (two per running task and
   per queue entry, bit 0 while the joiner waits); the unit that reaches
   zero signals the joiner. A task a worker took from a queue releases
   both units at its end with one atomic.
8. **Deadlock (XO-F001) is checked when a worker goes idle and when a root
   goes to sleep** (Go's checkdead), under the pool lock: every worker
   idle, no queued item, every root asleep in a counted wait, no context
   in an uncounted wait (timer, socket, signal). Spawn and task end no
   longer touch a global counter. The faults it reports are the same as
   0068's.
9. Hot words written by many threads (a scope's counts, a ring's head
   and tail, an Atomic's value apart from its count) sit on cache lines
   of their own (128 bytes on Apple cores).
10. `XO_RTSTATS=1` prints scheduler counters at exit (spawns, steals,
   wake ups, idle sleeps, spin rounds, fiber parks, new stacks, thread
   waits, slow locks).

## Consequences
- On 12 workers every conc benchmark uses less CPU and wall time than
  the Go backend (CPU 0.16x to 0.85x, wall 0.34x to 0.84x; spawn CPU 2.3 s
  to 0.25 s, ping pong 0.75 s to 0.045 s); one worker got faster too; the
  bench/perf/llvm compute programs are unchanged.
- What remains in spawn heavy code is reference counting a handle that
  every spawned closure captures (one shared count all workers update),
  not the scheduler.
- A root that helps may be busy with a long task when its own wait ends;
  it continues after that task parks or ends (no preemption).
- ThreadSanitizer: the goldens and the C tests are clean at 1, 2, and one
  per CPU workers; ThreadSanitizer itself crashes on Darwin after a few
  hundred thousand fiber tasks at one or two workers, also before this
  change at larger counts (`gotchas/tsan-fiber-trace-crash.md`), so TSan
  runs use reduced sizes.
- The Linux futex path compiles in principle but was not run (no Linux
  host, as for 0069).

## Addendum: deferred wakes and their monitor (2026-10-16)

Owner, 2026-10-16 ("Sysmon sounds like a good strategy"). The scheduler is in Xo now
(std/native/llrt/pool.xo).

Context: profiles (decision 0238) showed the park and wake paths paying
futex system calls where Go and tokio's current-thread runtime do not:
in bench/perf/conc pingpong every channel hand off woke an idle worker
(wakep: a futex wake, then that worker's spin and its futex wait), about
twice the CPU of a ping pong without them. Skipping the wake is safe
only if something bounds how long the woken work waits when its waker
keeps computing (no preemption, above); Go bounds it the same way, with
sysmon.

Decision:

1. A channel's hand off to a parked task (chan.xo hand,
   tasks.park_signal_handoff) puts the task in the waker's next slot and
   wakes no idle worker (pool.xo pool_push_handoff); the worker's
   W_DEFER_AT records when. A spawn whose task is the only item of the
   spawner's ring defers the same way (scope_add). The owner usually
   runs the work when its task parks or ends; take_local clears the mark.
   Mutex wakes are unchanged (they skip the next slot on purpose, for
   HTTP p99, mutex.xo signal).
2. A monitor thread (pool.xo mon_main), started on the first deferral,
   wakes an idle worker (wakep) for any deferral older than 1 ms whose
   work is still queued. It ticks every 1 ms while deferrals come and,
   after 10 ticks without one, sleeps on a futex with no timeout until
   the next deferral wakes it (once per idle period). One worker
   (XO_THREADS=1) defers nothing and starts no monitor. Its record is
   static word 46. XO_RTSTATS counts mon_tick, mon_wake, defer.
3. A worker that finds no work no longer spins before going idle: the
   default spin rounds are 0 (XO_SPIN_ROUNDS still sets them). 16
   rounds cost rwmutex 1.27x the CPU (its idle workers stole from each
   other) and bench/perf/http's p99 was the same at 0, 4, and 16
   (1,144, 1,142, and 1,148 us, medians of 5 interleaved runs).
4. The monitor does only this. Go's sysmon also retakes the processor
   of a task running too long (preemption), wakes the network poller,
   and forces garbage collection; those are future work here.

Consequences, measured on the Mac (bench/results/park-wake-2026-10-16.md,
both parts against main, medians of 9 interleaved rounds):

- pingpong 0.79x wall, 0.47x CPU (397M to 272M instructions); rwmutex
  0.86x wall, 0.80x CPU (784M to 620M); mutex 0.89x and 0.86x, at one
  worker 0.78x and 0.60x; spawn, fanout, and pmap within noise.
  bench/perf/http (the CRUD server, arm64, medians of 5): p50 234
  against 236 us, p99 1,140 against 1,152 us (ranges 1,137 to 1,154 and
  1,132 to 1,170), 97.4K against 97.5K requests a second: within noise.
- A task deferred behind a waker or spawner that keeps computing starts
  within about 1.3 to 2.1 ms (internal/llvm TestDeferredWakeBound, XO_THREADS=4,
  asserting under 10 ms), where an immediate wake started it in about
  60 us: the price of the hand off, paid only when the waker does not
  park soon.
- An idle program pays nothing: an idle HTTP server's monitor sleeps
  (90 ticks over a 63 s run with 11 deferrals, none during the idle
  minute; CPU time unchanged); XO_THREADS=1 starts no monitor (the same
  4 threads as before).

## Alternatives
- **Chase and Lev deques** (built first, commit 9198503): lock free for
  the owner, but a thief takes one item per CAS on the spawner's top, and
  batch stealing is unsafe with an owner that pops without a CAS; spawn
  CPU stayed 1.2x the Go backend.
- **Running main on a fiber** (Go's main goroutine): symmetric, but every
  program would switch stacks at start, the C tests drive the runtime from
  a plain thread, and a sleep in a single threaded program would start
  the pool. Roots that help give the same hand over.
- **Counting live and blocked contexts** (0068): exact at every change,
  but two global atomics per task that every worker contended for.
- **One spinner budget in time** (the earlier 4000 iteration spin with up
  to two spinners): burned CPU scanning deques under locks.
