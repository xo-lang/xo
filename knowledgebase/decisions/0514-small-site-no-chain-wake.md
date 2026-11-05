# 0514: On macOS a small spawn site's task wakes no chain worker

Status: Proposed (xo-lang-4, 2026-11-05; the code waits for the owner's
accept, branch l4-chainsmall fdfd6caa7)

## Context

micro13 spawn100k (100,000 tasks of one atomic add in a scope) on
darwin/amd64 (the owner's Intel laptop, 6 cores, 12 threads) ran 28 to 29 ms
of its own time against Go 1.26.1's 25 to 26 after c49659a70 made
Atomic.add one fetch and add; linux/arm64, linux/amd64 and darwin/arm64 were
at or ahead of Go. On the laptop fewer workers ran it faster
(XO_THREADS=2: 24 ms, 4: 26, the default 12: 29); spinning longer was
slower (XO_SPIN_ROUNDS 4 to 256: 32 to 33) and XO_CAP=auto did not help.
XO_RTSTATS per 10 runs: 35,316 idle sleeps and 41,216 chain wakes on the
laptop against 14,030 and 15,399 on linux/arm64. A profile of the laptop
run: 59% of samples in `_ulock_wait`.

Decision 0359 keeps a burst of a small site's spawns on the spawner (no
wake per spawn); but worker_main's chain wake (Go's rule: the last spinner
to find work wakes another spinner) still fanned the burst out: each woken
worker stole a batch, became the last spinner and woke the next, and each
macOS wake and sleep cost more than the tasks it carried.

## Decision

On macOS (pool.xo layer() == 2, a constant), a spinning worker that finds a
task of a small spawn site (site_small, decision 0359) does not wake another
spinner. Fibers and tasks of other sites keep the chain. Linux and Windows
keep it unchanged: on the 28 CPU linux/amd64 offload host the chain made
about 430 wakes a run whose steals carried the burst, and dropping it there
ran spawn100k 1.231 times as long (13 to 16 ms, 21 rounds, quiet), worse at
every XO_THREADS value from 4 to 28.

## Measurements

`xo bench --ab`, spawn100k, A = main ddd2ed554, B = the change, 21 rounds,
work_ms = the program's own ms:

| platform | work_ms | wall | cpu |
|---|---|---|---|
| darwin/amd64 (laptop, load 1.3) | 0.897 (29 to 26) | 0.926 | 0.641 |
| darwin/arm64 (Mac, load 7.0) | 0.818 (22 to 18) | better | 0.583 |
| linux/arm64, linux/amd64 | code unchanged | | |

Against Go 1.26.1 with the change: darwin/amd64 25 vs 27 (0.926, within
noise), wall 0.931, cpu 0.841; darwin/arm64 18 vs 27 (0.667), wall 0.766,
cpu 0.694.

The same rule on every OS (6cf6714c9) on the linux/arm64 ab-host, 11 rounds:
spawn100k wall 0.758, cpu 0.588; conc spawn 0.922; fanout 0.993, pingpong
1.005, pmap 1.013, mutex 1.003, rwmutex 0.992, httpsession 1.0 (noise).
The change on darwin/arm64 (Mac, std of main against the change, 11 rounds,
wall): conc spawn 1.006, fanout 0.997, pingpong 0.997, pmap 0.987, mutex
0.959 (better), rwmutex 0.95 (noise), httpsession 0.995 (noise).

Correctness: the 27 concurrency goldens (conc_, spawn, sched, task_,
scope_, fanout, chan_) pass on darwin/arm64.

## Consequences

- On macOS bursts of tiny tasks wake and steal less; idle workers stay
  asleep and CPU per run falls 36 to 42%.
- A site's first burst (no average yet) still chains.
- Liveness rests on decision 0359's monitor tick and the spawner's join, as
  for the spawns themselves.
- The scheduler differs by OS in one more place (after 0359's defer).

## Alternatives

- The rule on every OS (6cf6714c9): linux/amd64 1.231 worse.
- Chain only while fewer than 4 workers are awake (f930dc738): laptop 26,
  linux/amd64 15 vs 13.
- Chain while a score of woken workers finding work is not negative
  (5bd79124f): no win on the laptop (29).
- XO_CAP=2 by default, longer spinning: see Context.
