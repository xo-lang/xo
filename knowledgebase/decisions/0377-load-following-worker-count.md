# 0377: The awake worker count follows the load

Status: Proposed (Xo Lang 3, 2026-10-25, for the owner: it changes the default scheduler behavior).
2026-10-27: no adaptive variant is clean (see "Variants screened"); the default stays off and `XO_CAP=N` is the opt-in for
server deployments.
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

## Measured on 2026-10-26: the saving is real, but a cap on the awake count does not reach it

Today's main, bench/perf/http/xo (native arm64, c=32, 8 s, quiet via heavy.sh measure; load 3), the same
binary, three interleaved reps, XO_RTSTATS counts over the whole run:

| XO_THREADS | req/s | p99 us | CPU us/request | wakes | idle sleeps | steals |
|---|---|---|---|---|---|---|
| 12 (default) | 144,958 / 152,329 / 145,559 | 671 / 639 / 680 | 24.88 / 21.69 / 24.66 | 294,548 / 253,260 / 297,047 | 284k / 246k / 287k | 571k / 510k / 565k |
| 6 | 144,205 / 147,025 / 145,276 | 632 / 620 / 627 | 21.49 / 21.14 / 20.92 | 197k | 192k | 477k |
| 4 | 148,329 / 148,945 / 148,430 | 600 / 603 / 604 | 19.61 / 18.92 / 19.50 | 134k | 131k | 372k |

Median CPU a request 24.66 to 19.50 (-21%) at 4 workers and -14% at 6, with throughput equal or higher and
p99 lower, wakes -55%: the 0377 table stands on today's build.

The cap built to reach it (branch lw-cap: P_CAP, wakep refusing a worker while the awake count is at the cap,
shrink by empty wakes, grow when the pusher holds more than one item or a worker runs a slice with work
waiting; XO_CAP=N fixes it; counters cap_grow, cap_shrink, cap_hold, cap_seen) does nothing: fixed at 6 or 4
or adaptive, wakes (255k to 262k), CPU a request (22.1 to 24.1) and rps equal the default's. cap_hold, the
wakes the cap refused, is 64 to 83 of 260,000 wakep checks at XO_CAP=4: the awake count is below 4 almost
every time a push wants a worker (about 3.6 CPUs of work spread over 12 workers: each push finds a sleeper,
wakes it, and it sleeps after about a request). Adaptive: the cap shrank 46 to 67 and grew 44 to 64 times
in a run, so "grow when the pusher's ring holds more than one item" cancels every shrink. The saving is not
from fewer awake workers but from fewer workers taking part: with 4 workers the same load finds work in
the 3 other rings before any worker sleeps (steals per wake 2.8 against 1.9, idle sleeps halved).

### Built (2026-10-26, 283716490): the cutoff, opt in

P_CAP and a worker index (W_IDX, in start order; roots 0): wakep takes only an idle worker whose index is under
the cap and starts one only under it; XO_CAP=N fixes the cap (2 up to the CPUs), XO_CAP=auto follows the load,
unset or 0 is the CPUs (today's behavior, no cutoff). Counters cap_grow, cap_shrink, cap_hold (wakes the cutoff
refused), cap_seen. CRUD (bench/perf/http/xo, c=32, 8 s, quiet via measure, 3 interleaved reps, medians):

| config | req/s | p99 us | CPU us/request | wakes |
|---|---|---|---|---|
| default | 148.2k | 645 | 25.31 | 297k to 315k |
| XO_CAP=4 | 147.2k | 620 | 19.40 (-23%) | 131k to 134k |
| XO_CAP=6 | 146.0k | 624 | 21.27 (-16%) | 198k to 205k |
| XO_THREADS=4 | 142.6k | 650 | 18.89 | 122k to 133k |
| XO_CAP=auto | 147.0k | 646 | 24.57 (-3%) | 277k to 291k |

The fixed cutoff reproduces the XO_THREADS rows (cap 4 is a little better on rps and p99). The adaptive rule is
too timid so far: cap_grow 109 to 130 and cap_shrink 111 to 132 a run, the cap oscillating near the CPUs. Conc
benches (15 rounds, quiet, against the default): XO_CAP=4: pmap 1.776x wall (a CPU bound program held to 4
workers: instructions equal), fanout 0.994, spawn 0.993; XO_CAP=auto: pmap 1.008, fanout 1.007, spawn 1.000 (all
noise). Gates: TestRuntime*, TestRace*, TestTSan (260 s), the arm64 scheduler goldens with and without
XO_CAP=2, the fixed point from the seed (s2 = s3). The default is unchanged; flipping it, or tuning auto
until CPU bound programs keep every worker and CRUD gets the 4 to 6 worker row, comes back to the owner with the
numbers.

### Design to build, if the owner agrees: a worker index cutoff

P_CAP counts the workers that take part, as XO_THREADS=P_CAP does but changing at run time: the idle list
holds every worker; wakep takes only a worker whose index is below the cap (a scan of the list from its head,
at most 12 entries under the pool lock), start_worker starts one only below the cap, and a worker with an index
at or over the cap that finishes its item goes to the idle list and is not woken (steal rounds already skip
nothing, so the over-cap worker's ring is empty and stealing scans it for free). Grow: a wakep that finds
every worker below the cap awake, with the pusher's queue deep, raises the cap by one (so the first burst
gets its workers); the monitor tick raises it to the CPUs when a worker runs a slice with work waiting;
shrink: 64 wakes of which 32 found nothing lower it by one, not below 2 (as built). Same kill switch
(XO_CAP=0, XO_CAP=N). The prototype will first be run as a fixed cutoff (XO_CAP=4, 6) to check it reproduces
the XO_THREADS rows above, then adaptive; no default changes until the owner sees those numbers.

## Measurement plan (all by `xo bench --ab` or run.sh medians, 5 interleaved reps, quiet machine; never callgrind)

1. CRUD (bench/perf/http/xo, c=32, 8 s): req/s, p50/p99, CPU a request, wakes and futex wakes a request,
   against the default and against XO_THREADS=6 and 4 as the target. Also c=4 and c=256.
2. HTTP/2 (SCHEME=https H2=1, bench/perf/http run.sh) and TLS: flat req/s and CPU.
3. The conc twins (the concurrency benchmarks in bench/perf: sysio chan, ctx, atomic/contended): flat.
4. CPU bound: pmap (tasks over a list), nbody and a handler that computes for 1 ms: wall within noise
   against the default, the cap growing to ncpu in the counters (add a `cap_grow` counter to XO_RTSTATS).
   pmap (bench/perf/conc/xo/pmap) is the guard, and its gap to Go splits in two (instructions and
   cycles from /usr/bin/time -l, 2026-10-26, load ~43, ratios only): the kernel's overflow checks
   (`3 * n`, `c += 1`: +25% instructions, +4% cycles single threaded, 2.04 G against Go's 1.96 G;
   not this decision's) and the coordination cost (Xo's parallel run burns 4% more cycles than its
   own serial run, 2.12 G against 2.04 G, where Go's parallel equals its serial): the part this
   decision can move. Expect pmap's cycles to fall toward the serial figure, never above today's.
5. The bursty case: a server idle for a second, then 1,000 requests at once: first-burst p99 against the
   default.
6. The sweep (bench/perf/sweep.py) for instructions of the scheduler benches, and scheduler goldens on
   arm64, LLVM and under XO_THREADS=1, 2.

Reject if the CPU bound rows or the burst p99 regress beyond noise, or CRUD CPU a request does not fall.

## Variants screened on 2026-10-27 (Xo Lang 3; none is clean, the default stays off)

The owner conditionally approved a default of a fixed cap (max(2, ncpu/3), raised by the monitor when
work waits). The conditions were no regression on CPU bound fan-out, the self build or the fib edit.
All variants sit in one binary selected by `XO_CAP`; every row is `xo bench --ab` against `XO_CAP=0`
on the same binary (12 CPU Mac, 8 performance cores, fixed cap 4 = ncpu/3). The fast stand-in
(a cold bench/perf/http build) was calibrated first and rejected: fixed cap 4 cost +8.9% there against
+26% on the 20 round self build, under the +15% bar.

| Variant | fan-out micros | self build | CRUD c32 CPU/req | CRUD c128 CPU/req |
|---|---|---|---|---|
| fixed cap 4 (mode 1) | not run | 1.265 worse (20 rounds) | -16% | -25% |
| third + monitor slice raise (mode 3) | pmap 1.893 worse | 0.997 | not run | not run |
| delay raise, D = 20 or 50 us (mode 4, first form) | pmap 1.020 / 1.017 worse | 0.995 / 1.003 | flat | not run |
| searcher limit 1, no cap (mode 5) | within noise | 1.002 | flat (+4%) | not run |
| idle rate window 2 ms (mode 4) | pmap 1.086 worse | 0.997 | not run | not run |
| idle rate window 0.5 ms (mode 4) | pmap 1.041 worse | not run | -14% | -2% |
| same, raising at the first blocked wake while no worker has gone idle yet | pmap 1.024, spawn 1.040 worse | not run | -17% | -4.6% |

What the counters (`XO_RTSTATS=1`) showed:

- A fixed cap saves CPU by cutting wake and park cycling (wakes 325k to 147k, idle sleeps -54%, futex
  wait and wake -50%), not by spinning less; the Tokio rule (wake only if nobody is searching) is already
  the baseline, and Go's half-the-busy-workers spin cap changes nothing at the default of 0 spin rounds.
- Mode 3's slice raise never fires on a short CPU bound fan-out, so a capped pmap runs 1.89x slower.
- A raise on "work waited D us" fires on request servers within the first wakes, because tasks at the cap
  routinely wait longer than 20 to 50 us, and the cap then stays at the CPUs. Queue depth does not
  separate the cases either: CRUD at c128 queues 64 or more items at cap 4. A gap since the last worker
  went idle fires on CRUD bursts. A rate of workers going idle (a request server at the cap idles a worker
  every 50 us; a fan-out keeps them all busy) separates them at c32 (-14%) but not at c128, where the
  capped workers are nearly saturated and the cap still did not cost throughput.
- Any wait before raising costs a few ms on a 60 ms CPU bound run (pmap +4% at 0.5 ms, +8.6% at 2 ms);
  raising at the first blocked wake with no wait still leaves pmap +2.4% (the idle counter, the checks, and
  decay cycles), so the start-up wait was 1.7 of the 4.1 points.

Opt-in stands: `XO_CAP=N` (2 up to the CPUs) for a server deployment gives -16% (c32) to -25% (c128)
CPU a request with p99 equal or better and 1% fewer requests a second; it must not be set for build
tools or CPU bound batch programs (the compiler's own build is 26% slower at N = 4).

## Alternatives

- Lower XO_THREADS default: rejected, regresses CPU bound programs.
- More spinning (Go's way): measured worse on the Mac (decision 0324, and XO_SPIN_ROUNDS=16 above).
- wakep only when the pusher holds more than one item (experiment 2 of the parking note): cheap, run as
  a data point; it removes some wakes (wake_push 0.30 a request) but not the workers' sleeping, so it
  may get part of the win without a new mechanism.
