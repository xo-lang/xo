# 0359: A burst of spawns stays on the spawner (no wake per spawn)

Status: Proposed (Xo Lang 3, 2026-10-23; for the owner's decision with the
measurements below). Nothing is implemented in main.

## Context

Intel field report on 0.26.0: spawning 100k tasks is 1.25x Go. On the Mac
the conc `spawn` row (200 rounds of 1,000 tasks that add to an Atomic) is
1.21x Go's wall, and `rwmutex` 3.9x. `XO_THREADS=1` runs the same `spawn`
program in 20 ms against 59 ms with 12 workers (0.39x of Go's), so the cost is
cross-thread, not the work.

`xo trace` of `spawn` (39 ms): the spawner (main) runs 29.6 ms of it in
three long slices, about 148 ns per spawn against about 100 ns alone; the
four stealing workers are idle half the time; 12k futex wakes and 8k pool
lock slow paths per 200k spawns. Every spawn after the first in a ring calls
`wakep` (pool.xo scope_add_at: the wake is deferred only while the spawn is
the ring's only item), so a loop of tiny tasks keeps waking and feeding
other threads that finish them faster than the wake costs.

Refuted as the cause (each by `xo bench --ab`, 20 rounds, load under 6, spawn
wall vs main): waking a chain worker only when work exists (0.991); deferring
the wake until the ring holds 4 (1.011) or waking every 8th item (1.053);
raising the scope prune bound from 64 to 1024 (1.107); dropping the
node_link write into the previous sibling's cache line (0.982 vs the
bound). The allocator is not hot (1 sample of about 50 on the spawner).

## Decision (proposed)

`scope_add_at` never calls `wakep` for a spawn: the new task joins the
spawner's ring and the existing deferral wakes a worker if the spawner has
not run it within the deferral bound (50 us on the next spawn, 1 ms at the
monitor's tick). The spawner runs the burst itself at its join (as
`XO_THREADS=1` does), and workers steal what is left when they come.

## Measurements (variant G; `xo bench --ab`, 20 rounds, load about 5.5, arm64)

Against current main (wall; instructions in brackets):

| row | wall | |
|---|---|---|
| spawn | 0.329 (60.5 to 19.9 ms) | instr 0.641 |
| fanout | 0.547 | instr 0.847 |
| mutex | 0.671 | instr 0.703 |
| rwmutex | 0.374 | instr 0.185 |
| pingpong | 1.003 | tie |
| pmap | 1.298 worse (92 to 120 ms) | instr 1.000 |

The CRUD HTTP server (bench/perf/http, 32 connections, 4 interleaved runs):
rps 146.7k against 144.0k (medians), p99 685 us against 751, p99.9 1.2 ms
against 1.4: no loss (a handler task spawns from a worker whose accept loop
runs the next accept at once).

The cost is start latency of long tasks: pmap spawns 64 tasks of about 1 ms
per round from the main thread and joins; the workers that every spawn used
to wake now start at the deferral (50 us to 1 ms) and then ramp up through the
wake chain, one step per task. A variant with a 1 ms deferral bound (H) has
the same shape (spawn 0.53, rwmutex 0.28, pmap 1.264).

Two refinements were tried and each gives back one side:
- I: also wake a worker at scope join when the joiner's ring holds 2 or more:
  spawn 0.945 (noise), fanout 0.80, rwmutex 1.00, pmap 1.115. The wins mostly go.
- K: wake a worker when an inline task at join ran longer than 20 us and the ring is non
  empty: spawn 0.303, fanout 0.428, mutex 0.962, rwmutex 0.98, pmap 1.354.
  pmap is worse, not better: one extra wake does not rebuild the
  per-spawn fan out.

## Consequences

- Programs that spawn many tiny tasks (a loop of handlers, fan out of small
  reads, lock heavy rows) get 1.8x to 3x; their gap to Go closes (spawn from
  1.21x to about 0.4x of Go's wall, rwmutex from 3.9x to about 1.5x).
- A batch of long CPU tasks spawned in one burst from a single task starts its
  workers later, up to the deferral bound plus the wake chain's ramp up:
  pmap (64 tasks of 1 ms) is 30 percent slower. A program that spawns work
  with long tasks and joins at once is the hurt case; `task.map`-like
  helpers can wake workers explicitly (a hint) if the owner wants both.
- Needs a TSan/--explore pass (wake timing changes) and the full conc row set
  re-measured in a quiet window before it lands.

## Alternatives

- Keep the per-spawn wake (status quo): fastest ramp up for long tasks,
  1.2x Go on spawn, 3.9x on rwmutex.
- A size hint (`s.spawn_heavy` or a scope option) that wakes per spawn: keeps pmap,
  makes the default the burst of small tasks; needs API and spec.
- Estimate a task's size from the last run of the same spawn site: adaptive, no
  API, more runtime state; not tried.
- G for spawners that are the main thread only: not tried; main is where the
  measured rows spawn from.
