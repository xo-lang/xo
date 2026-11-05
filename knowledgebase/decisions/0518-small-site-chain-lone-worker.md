# 0518: On macOS a lone worker still chains for a small site's task

Status: Proposed (xo-lang-4, 2026-11-05; the owner decides). Amends
decision 0514. The rule is on main since 59ce17584 (std/native/llrt/pool.xo
worker_main), landed for 0.40.0 at the lead's request for a rule keeping
0514's gain without its conc spawn loss.

## Context

Decision 0514 stopped the chain wake on macOS when the last spinner finds
a task of a small spawn site. Its measurement was micro13 spawn100k, one
scope of 100,000 tasks. Xo Lang 2's 0.40.0 pre-cut found bench/perf/conc
spawn (200 scopes of 1,000 tasks) slower on darwin/arm64 than 0.39.0:
wall 1.191, CPU 2.144 (9 rounds). With only 0514's condition removed from
main's std, 9 rounds: wall 1.24 (1.1 to 1.251), CPU 2.182; this is 0514.

XO_RTSTATS of one run, without 0514 against with it: chain wakes 9 and 2,
but all wakes 7 and 55, idle sleeps 11 and 57, inject queue pops 46 and
1,029, inject lock waits 12 and 209. In conc spawn the chain is cheap: it
kept one spinner alive, and a live spinner makes every other wakep return
at once. Without it each scope's spawns (scope_add_at), the root's join
(root_hand_off) and full rings (the inject queue) woke sleeping workers,
each a macOS wake and sleep. In spawn100k the chain itself is the cost
(about 4,000 wakes a run on the Intel laptop).

## Decision

0514's rule applies only while at least two workers are awake (pool.xo
awake): the last spinner that finds a small site's task while it is the
only worker awake still wakes one more. Linux and Windows are unchanged
(they keep the chain).

## Measurements

`xo bench --ab`, 3 rounds unless noted; no514 = main's std without 0514's
condition, main = 0514, v2 = this decision; each built by the xo and std of main
c761d4ab0 (only pool.xo's condition differs).

darwin/arm64 (the Mac, load 4 to 5):

| program | no514 vs main | no514 vs v2 | main vs v2 |
|---|---|---|---|
| conc spawn wall | 1.24 (noise, n=3) | 0.939 (noise) | 0.881 better (11 rounds) |
| conc spawn CPU | 2.35 | 0.952 | 0.56 (11 rounds) |
| spawn100k wall | 0.851 better | 0.935 better | 0.953 (noise) |
| spawn100k CPU | 0.564 better | 0.604 better | 1.01 (noise) |

main vs v2 on the rest of the darwin/arm64 conc suite: fanout 1.005 and
rwmutex 0.982 (11 rounds, noise; their 3 round runs read 1.364 and 1.113),
pingpong 0.688 and pmap 0.83 (noise), mutex 0.998 (noise).

darwin/amd64 (the Intel laptop, load 1.4):

| program | no514 vs main | no514 vs v2 | main vs v2 |
|---|---|---|---|
| spawn100k work_ms | 0.893 (28 to 25) | 0.897 (29 to 26) | 1.04 (noise) |
| spawn100k CPU | 0.639 | 0.648 | 0.997 (noise) |
| conc spawn wall | 0.964 better | 0.946 better | 1.013 (noise) |
| conc spawn CPU | 0.629 | 0.609 | 1.006 |

## Consequences

- 0514's spawn100k gain stays on both Macs; conc spawn on darwin/arm64 is
  back at 0.39.0's time (21.7 ms) or better.
- worker_main reads P_ALIVE, the deadlock loop count and P_NIDLE once more
  on macOS when a spinner finds a small site's task.

## Alternatives

- Revert 0514: loses spawn100k (darwin/arm64 0.851, darwin/amd64 0.893).
- Cut the chain only when the spinner took a batch into its own ring:
  spawn100k 0.922 (noise), conc spawn 1.1 (noise), CPU gain mostly gone.
- 0514's own alternative, chain while fewer than 4 workers are awake:
  the laptop ran spawn100k 26 ms, as here, but that was measured on every
  OS (linux/amd64 15 vs 13); here the rule is macOS only.
