# bench/perf/conc rwmutex and pingpong against Go (2026-10-23)

`xo bench --ab --rounds=10`, A = Go twin, B = Xo, gate class, 1 minute load
5.9 and 5.7 at the start, 0 loaded rounds (agents/ccab2-cmd.log):

| row | wall | instructions | cpu_ms |
|---|---|---|---|
| rwmutex (64 tasks x 4,000 accesses, a quarter writes) | 3.90x (3.70 to 4.10) | 15.6x | 10 to 110 |
| pingpong | 1.77x (1.70 to 1.88) | 1.35x | 10 to 30 |

## Where the time goes (rwmutex)

- Threads: Xo with `XO_THREADS=1` takes under 5 ms and 131 M instructions
  (Go at `GOMAXPROCS=1` 57 M); with 2 threads 1.16 G instructions and 40 ms,
  with 4 threads 1.52 G and 40 ms (Go at 4: 95 M). The gap exists only with
  more than one worker.
- Parks: `XO_RTSTATS=1` counts 247,713 Mutex parks (`sig_mutex`) for 256,000
  accesses, about one per access, 63k wake pushes (the writes). Few idle
  wakes (73), steals (8) and pool lock slow paths (554). A Go twin with
  `SetBlockProfileRate(1)` blocks about 2,700 times (1%) at `GOMAXPROCS=4`.
- Go's wall is the time of about one core: 9.8 ms and 0.00 cpu_ms with a
  start barrier (64 tasks wait on one closed channel), 0.12 to 0.26 s with
  0.18 s of user time at 20x the work, against Xo's 59.5 ms and 0.77 s with
  2.4 s of user time. The barrier row is 6.1x wall and 22x instructions
  against Go, so the gap is not a staggered start.
- A macOS `sample` of the running threads showed clock reads
  (`mono_ns` through gettimeofday) as most of the samples; fixed by
  mono-clock-2026-10-23.md (rwmutex instructions 0.805, wall within noise).

## Tried

- A bounded spin on the state word before a reader or writer parks
  (compile-time 10, 50 and 250 rounds of 4 spin hints, `--ab` against 0):
  rwmutex 1.02, 1.04 (noise) and 1.44x wall; mutex 1.07, 1.31 and 3.9x wall
  (instructions 1.14, 1.62, 5.06x); pingpong unchanged. Negative at every
  length; the default stays 0.
- Blocking readers only behind the head-of-line writer (Go's structure) was
  considered and dropped: Go's `RWMutex.Lock` also blocks new `RLock`s while
  a writer is pending; the difference is how many tasks run at once.

- The grace windows (pool.xo GRACE_NS, the reader grant's, and
  NEXT_GRACE_NS, the run-next slot's), compile-time, `--ab` against current
  main, load 5, rows rwmutex, pingpong, mutex, pmap, fanout: GRACE_NS 2 us
  makes rwmutex 1.26x wall (worse), 200 us noise; NEXT_GRACE_NS 10 us and
  500 us noise on every row (pmap 10 us 1.34x with an interval of 0.69 to
  1.84). Neither knob is the lever; the default stays.

## Reading

This row is lock bound: one task at a time can make progress, so extra
workers only add contention, and Xo spreads the 64 tasks over all workers
at once where Go's goroutines stay with the waker's P. The CRUD server
(a lock held for a few microseconds between socket waits) is the workload
that matters; there Xo is at 0.93x to 1.0x of Go's requests a second
(decision 0118's addendum). Open candidate, `--ab` only: throttle
worker wakeups and steals for a program whose parks are all Mutex parks
(rtstats shows few wakes and steals already: 73 and 8 for 256k accesses, so
the contention is among the workers that are awake, not from waking more).
