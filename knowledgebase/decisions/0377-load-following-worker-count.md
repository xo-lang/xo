# 0377: The awake worker count follows the load

Status: Proposed (Xo Lang 3, 2026-10-25, for the owner: it changes the default scheduler behavior).
Nothing is built. Measured evidence below; the design and the measurement plan are the proposal.

## Context

The profile of the CRUD server under load (bench/results/crud-profile-2026-10-25.md; bench/perf/http/xo,
native arm64, c=32, 12 CPUs, 147k requests a second) puts 10.8% of its CPU in parking: `_ulock_wait` 6.2%
and `_ulock_wake` 4.6%, on top of 81% in the read and sendto syscalls and under 3% in Xo code. XO_RTSTATS
counts per request: 0.20 worker wakes (wakep took an idle worker), 0.28 futex wakes, 0.175 idle sleeps,
1.67 fiber parks, 0.32 steals. A sleeping worker sleeps about one request (200 us): less than a futex round
trip repays. Most pushes wake a worker for an item the pushing worker (or a spinner) would reach first.

The worker count is a fixed one per CPU (XO_THREADS to change it). Five interleaved reps on a quiet
machine (load 1) with the same binary:

| XO_THREADS | req/s (median, range) | p99 us | CPU us/request | wakes/request | futex wakes/request |
|---|---|---|---|---|---|
| 12 (default, one per CPU) | 147,056 (146.7k to 148.7k) | 649 | 25.5 | 0.207 | 0.280 |
| 6 | 145,798 (144.7k to 148.9k) | 617 | 21.3 | 0.137 | 0.188 |
| 4 | 146,572 (139.3k to 148.2k) | 619 | 19.8 | 0.092 | 0.133 |

Same throughput (the closed loop bench is client bound) and p99, 16% less CPU a request at 6 workers and
22% at 4, a third and a half fewer wakes. A lower fixed default would regress a CPU bound program (a
parallel map, nbody, a CPU heavy handler) that needs every core. Decision 0324 found that capping the pool
did not recover its 6 core case on the Docker runs (there the counters followed the free cores the threads
landed on); on this Mac with the load generator on other cores it does.

What was measured and is not the answer: spinning (XO_SPIN_ROUNDS=16: 134k req/s, 0.235 wakes a request,
worse), handing a Mutex grant off through the deferred run-next slot (rejected by decision 0077's
addendum: it held the next waiter 1.3 ms behind a holder that kept computing).

## Go's rule, and how this differs

Go wakes an idle P (wakep) only when no M is spinning and an idle P exists, and lets a spinning M stop
spinning when `2 * nmspinning >= gomaxprocs - npidle`: spinners are capped at about half of the busy Ps.
Xo's wakep has the first (P_SPINNING) but not the second, and workers do not spin by default (spin rounds
0, decision 0077's addendum), so an idle worker sleeps at once and the next push wakes it: the churn above.
Go keeps Ms from sleeping by spinning, which costs CPU on the Mac (decision 0324: 7% more CPU a request);
this proposal keeps fewer workers awake instead.

## Decision (proposed)

1. A cap on the awake workers, P_CAP, from 2 up to the worker count (ncpu), starting at ncpu: wakep and
   start_worker take a new worker only while `workers(p) - idle < cap`.
2. Shrink: when a worker that wakep woke finds nothing of its own to run (it goes back to sleep without
   running an item; idle_sleep right after wake), count it; at 32 such wakes in a row of 64 the cap falls
   by one (not below 2), and the count resets. A stable load that needs k workers settles at k plus slack.
3. Grow, at once: wakep that finds `workers - idle >= cap` and the pushing worker's ring (or the inject
   queue) holding more than one item raises the cap by one and wakes a worker (the push has more work than
   the awake workers' next items). The monitor's tick (preempt_arm) raises it to ncpu when a runnable item
   waits past a slice (the existing tick exists for preemption). So a CPU bound program that spawns many
   tasks gets its workers on the first push that sees a queue, and a worker stuck in a long computation
   is bypassed by the tick.
4. Every other behavior (the poll token, the grace, steals, timers) is unchanged; the deterministic
   (det.built) and single thread builds never see the cap.

## Consequences

- Expected: CRUD CPU a request toward the 6 worker row (-16%) with throughput and p99 flat; a CPU bound
  program unchanged (its first spawn raises the cap to the work).
- A burst after a quiet period starts with a low cap: the first push with a queue raises it to a worker
  per queued item pair, each a futex wake as today, so the first burst's latency is the same as now.
- The cap is one more word of scheduler state and two branches in wakep; the shrink counter is plain
  words under the pool lock.

## Measurement plan (all by `xo bench --ab` or run.sh medians, 5 interleaved reps, quiet machine; never callgrind)

1. CRUD (bench/perf/http/xo, c=32, 8 s): req/s, p50/p99, CPU a request, wakes and futex wakes a request,
   against the default and against XO_THREADS=6 and 4 as the target. Also c=4 and c=256.
2. HTTP/2 (SCHEME=https H2=1, bench/perf/http run.sh) and TLS: flat req/s and CPU.
3. The conc twins (the concurrency benchmarks in bench/perf: sysio chan, ctx, atomic/contended): flat.
4. CPU bound: pmap (tasks over a list), nbody and a handler that computes for 1 ms: wall within noise
   against the default, the cap growing to ncpu in the counters (add a `cap_grow` counter to XO_RTSTATS).
5. The bursty case: a server idle for a second, then 1,000 requests at once: first-burst p99 against the
   default.
6. The sweep (bench/perf/sweep.py) for instructions of the scheduler benches, and scheduler goldens on
   arm64, LLVM and under XO_THREADS=1, 2.

Reject if the CPU bound rows or the burst p99 regress beyond noise, or CRUD CPU a request does not fall.

## Alternatives

- Lower XO_THREADS default: rejected, regresses CPU bound programs.
- More spinning (Go's way): measured worse on the Mac (decision 0324, and XO_SPIN_ROUNDS=16 above).
- wakep only when the pusher holds more than one item (experiment 2 of the parking note): cheap, run as
  a data point; it removes some wakes (wake_push 0.30 a request) but not the workers' sleeping, so it
  may get part of the win without a new mechanism.
