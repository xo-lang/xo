# 0640: a root thread that helps run tasks takes one of the pool's worker slots

Status: Accepted (the lead chose option (a), 2026-11-14; Xo Lang 2)

## Context

Decision 0638 step 2 sizes the pool from the process's cgroup CPU quota, as
Go's GOMAXPROCS does. Its p99 measurement (bench/perf/http, c=128, the
linux/amd64 offload host, a systemd scope with CPUQuota=200%) found Xo's tail
at 13 to 25 ms against Go's 5 to 8 ms while Xo served more requests. The
cause (notes TAIL-PLAN.md, with the scope's cpu.stat and each thread's
/proc stat):

- with XO_THREADS=2, three threads each used about 7.6 s of CPU in a 12 s
  run: the two pool workers and the root (main's) thread, which runs tasks
  while main waits (decision 0077, root_main);
- under the quota of 2 CPUs those three threads used the period's budget
  early and the cgroup stalled every thread until the next 100 ms period:
  99 to 119 throttles, 5.6 to 7.4 s throttled; Go: 0 or 1;
- pinned to 2 CPUs with no quota the same build's p99 was about 4.0 ms at
  115k to 132k requests a second, better than Go's 5.1 ms at 88k;
- XO_THREADS=1 under the quota: two busy threads, 0 throttles, p99 6.8 ms.

So XO_THREADS=N ran N + 1 threads of Xo work, and a quota of N CPUs
throttled. Go's model is the reverse: GOMAXPROCS=N is N Ps, and a thread runs
Go code only while it holds one.

## Decision

The root's loop counts toward N. The pool starts and wakes workers only up to
N less the roots in their loop (pool.xo worker_cap: XO_THREADS, or the
CPUs, or the quota's count, minus DLK_LOOPS), never below one worker. Every
check of "every worker busy" (wakep, root_hand_off, the monitor's waiting
test) uses the same count. The root is on the idle list like any worker, so
wakep wakes it for its slot.

XO_THREADS=N now means at most N threads running Xo tasks for N >= 2. N=1
keeps one worker beside the root (two threads, as before): with no worker a
root blocked in the system (a file read, a C call) would stall every task,
and Xo has no handoff of a blocked thread's slot (Go's sysmon retakes a P
from a thread in a syscall). The quota's count is at least 2 (Go's rule,
decision 0638 step 2), so a quota run always gets the N-thread form.

The tests that pin it: internal/llvm TestRuntime*, TestOneThread*,
TestRoot* and TestProfSpawnStress (one worker beside the root, its hand
off and its spin), with the decision's thread counts read from /proc on
the measurement host.

Not covered: a thread that is not a root in its loop but helps while it waits
(park_wait with f == 0: a foreign thread, a `use c` callback) still runs
tasks beside the N; it is rare and short.

## Consequences

Measured on the linux/amd64 offload host (28 CPUs) under a systemd scope
with CPUQuota=200% (cpu.max 200000 100000; the quota's count is 2), base
the 0638 step 2 build, new the same with this decision, Go 1.26.11 with
GOMAXPROCS following the quota, bench/perf/http's CRUD server, 10 s of
load after 2 s of warmup, 3 repetitions (notes fixa/quota-summary.md):

| Connections | Build | Requests/s | p99 | Throttles (time) | Busy threads |
|---|---|---|---|---|---|
| 128 | base | 105k to 107k | 25.0 to 26.1 ms | 119 (7.3 to 7.8 s) | 3 |
| 128 | new | 129k to 130k | 3.94 to 3.97 ms | 2 to 3 (1 to 2 ms) | 2 |
| 128 | Go | 87k to 90k | 4.93 to 5.15 ms | 0 | 4 |
| 32 | base | 76k to 78k | 3.99 to 4.06 ms | 86 to 92 | 3 |
| 32 | new | 79k to 83k | 3.46 to 3.71 ms | 0 | 2 |
| 32 | Go | 70k to 72k | 3.09 to 3.12 ms | 0 | 4 |
| 8 | base | 33k to 35k | 0.93 to 0.98 ms | 0 | 3 |
| 8 | new | 33k to 36k | 0.98 to 1.05 ms | 0 | 2 |
| 8 | Go | 34k to 35k | 0.88 to 0.92 ms | 0 | 4 |

The CPU per request falls at every load (8 connections: 33 to 35 us from
42 to 46). At 8 connections the p99 is about 7% higher, within the spread
of three runs.

With no quota, `xo bench --ab` on the linux/arm64 offload host (12 CPUs;
11 workers and the root against 12 and the root): xo's own build with
xos built from each side (stage2) 1.001 wall, 0.999 CPU, within noise;
bench/perf/alloc 0.95; conc spawn, pingpong and fanout within noise;
httpkeep wall within noise but 1.095 CPU, and httppost 1.05 at the edge of
its noise (22 ms runs). Both follow the thread layout, not this change:
the base run at XO_THREADS=11 (11 workers and the root) uses the CPU of
the new default (httpkeep 20.21 against 20.07 s), and the new run at
XO_THREADS=13 that of the base default (18.12 against 18.04 s), wall 1.84
to 1.86 s in all four; httppost's medians over 30 runs a side at matched
layouts are 23.4 against 24.4 ms and 23.8 against 23.7 ms, interquartile
ranges 22 to 26 ms. Twelve threads on 12 CPUs give the spinners time the
OS took from them with 13 (XO_RTSTATS: chain wakes 2.5 times, spin rounds
1.27 times).

Residual (follow-up): the 2 to 6 throttles a run that remain, about 1 to 8
ms in 12 s, are the scheduler's monitor thread (mon_main). Sampled each 100
ms period, two threads run at 96% and the monitor adds about 0.16 s a
run, waking every 1 ms (DEFER_US) while deferred wakes or waiting work
exist; the throttles fall in periods where the three together pass 200
ms. XO_DEFER_WAKE=0, which never starts the monitor, gives 0 throttles in
three runs with the same throughput and p99; XO_PREEMPT=0 alone still
throttles (the deferral checks keep it ticking). Turning deferral off
under a quota is not the fix, since it also ends the preemption of a CPU
bound fiber. The follow-up models the monitor on Go's sysmon (a sleep of
20 us doubling to 10 ms after 50 idle cycles, preemption only of a fiber
past its 10 ms slice, a full park when nothing runs), with 0 throttles
under the quota as its target and an --ab of the deferral latency it
serves.

## Alternatives

- Defer wakes off under a quota (no monitor thread): 0 throttles, but no
  preemption of a CPU bound fiber either; the residual's follow-up instead.
- (b) The quota default counts the root (workers = limit - 1 when main
  helps), leaving XO_THREADS=N as N workers plus the root. Rejected by the
  lead: XO_THREADS would keep meaning something other than Go's
  GOMAXPROCS, and a user's XO_THREADS=2 under a 2 CPU quota still throttles.
- A token the root takes before each item (a P handed between threads):
  exact under every path (park_wait's helpers too) but adds an atomic pair to
  every item the root runs; the count at start and wake bounds the threads
  with no cost on the run path.
- The root stops helping (decision 0077 reversed): main's waits would cost a
  thread switch each, which 0077 measured as the reason to help.
