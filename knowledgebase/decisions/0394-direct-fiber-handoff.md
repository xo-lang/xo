# 0394: A park hands off straight to the fiber its wake queued

Status: Accepted (Xo Lang 3, 2026-10-28; the lead approved the work, the 0.34 milestone's criterion 4, ping-pong).

## Context

The channel ping-pong (bench/perf/conc/xo/pingpong: two tasks passing an Int over two unbuffered channels)
is 1.20 of Go's wall time in the 0.33.0 comparison (steady state at 10M iterations: Xo 2.13 s, Go 1.70 s). A
handoff is a send that wakes the partner and a receive that parks. In Xo each park is two stack switches: the
fiber to the worker's scheduler, then the scheduler to the woken fiber, which the waker left in the worker's
run-next slot. Of 216 ns an iteration (4 switches), the profile has xo_stack_switch at 27%, the park
and run bookkeeping (pool.park_wait, run_fiber, tasks.park_wait_n) at about 27%.

## Decision

A fiber that parks while its worker's run-next slot holds a fiber (the one its wake queued) switches straight
to it: one stack switch, no scheduler hop. The fibers' state handshake is kept: a parking fiber's state moves
from PARKING to PARKED only after it has left its stack (so a waker cannot start it on another worker while
this one is still on it), now done by the fiber it switched to, on its first instructions (sched.xo
finish_prev; the worker record's W_PREV names the fiber); a fiber woken while it switched is queued again on
the worker's ring. The scheduler loop (fiber_run) learns which fiber switched back (a chain of direct
switches may end on another), W_LASTF.

Fairness: a pair handing off must not hold a worker from its ring, its timers, or the poller. Go's runnext
handoff inherits the time slice for this reason. Here the direct switch does not restart the slice (W_RUN_AT_I
is set only by run_fiber); every 64th direct switch reads the clock and goes through the scheduler once the
slice (SLICE_NS, 10 ms) has run out; none happens while the monitor has asked this worker to yield; the ring
must have room for the requeue.

Not taken: when the fiber is yielding (preemption) or already woken; when the slot's item is not a fiber;
when scheduler hooks are on (XO_RTSTATS=1, --race, the execution trace: F_HOOKS), so those builds keep the
old path and their tests stay valid; in deterministic mode (no run-next slot there). XO_DIRECT=0 turns it off.

## Consequences

Measured (`xo bench --ab`, 20 rounds, quiet, 12 CPU Mac):

- ping pong, 2M iterations, one binary, `XO_DIRECT=0` against the default: wall 0.698 (0.694 to 0.702), CPU 0.69,
  instructions -8%. Against Go: 0.864 (0.856 to 0.869); with `XO_DIRECT=0` it was 1.244 (1.237 to 1.26).
- bench/perf/conc, baseline xo against this one: pingpong 0.734 (0.723 to 0.746), 0.745 (0.741 to 0.752) after the flag moved into the pool record; fanout 1.014, mutex 1.010,
  pmap 1.001, rwmutex 1.001, spawn 1.006, all within noise.

Fairness. The golden `direct_fair` (one worker: a ping pong pair, a CPU bound task, a timer) found that a pair
handing off through the run-next slot starves everything else on its worker, with or without this change:
`find_work` took the slot and the ring before the inject queue and the poller, so a 20 ms timer fired after 124 s
on the old path (633 s with the direct switch, which does not restart the slice). `find_work` now takes the ring,
the inject queue and the poller (timers, the network) before the slot once a slice (SLICE_NS) at most, checking
the clock every 64th call or when a direct switch ran out the slice; the slot's fiber stays there. The timer
now fires in 41 to 50 ms on both settings. gotchas/run-next-pair-starves-timers.md.

Gates: the 28 concurrency, channel, select, timer and preemption goldens on LLVM, `--race` runs of
conc_channels, conc_select, conc_stress, conc_send_closed, conc_scopes and direct_fair (hooks on keep the old path),
the fixed point (s2 and s3 identical), the Docker amd64 sample (48 of 48). Not measured: bench/perf/http.

Windows: the fibers share pool.xo and sched.xo; the next Windows CI run should cover it (W_RUNF, which the
monitor's thread reads for asynchronous preemption, follows the direct switch).

## Alternatives

- A switch that saves only fp and lr (Go's gogo): the callee-saved registers belong to the callers of the
  parking function, so every function that can switch would have to be treated as clobbering them: measured
  ceiling 4 to 5% (isolated: full switch 10.7 ns, minimal 6.0 ns), after this decision it halves.
- Inline the Darwin thread-local read (about 12 ns an iteration, 5%): a separate change.
