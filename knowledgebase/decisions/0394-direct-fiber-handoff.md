# 0394: A park hands off straight to the fiber its wake queued

Status: Proposed (Xo Lang 3, 2026-10-27; the lead approved the work, the 0.34 milestone's criterion 4, ping-pong).
Implementation in progress; the numbers follow with the code.

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

To be filled with the measurements (ping-pong, the concurrency rows, HTTP) and the gates (concurrency goldens
on arm64 and in the Docker amd64 sample, a --race run of the channel goldens, the fixed point, a fairness test
with a ping-pong pair, a CPU-bound fiber and a timer on one worker).

Windows: the fibers share pool.xo and sched.xo; the next Windows CI run should cover it (W_RUNF, which the
monitor's thread reads for asynchronous preemption, follows the direct switch).

## Alternatives

- A switch that saves only fp and lr (Go's gogo): the callee-saved registers belong to the callers of the
  parking function, so every function that can switch would have to be treated as clobbering them: measured
  ceiling 4 to 5% (isolated: full switch 10.7 ns, minimal 6.0 ns), after this decision it halves.
- Inline the Darwin thread-local read (about 12 ns an iteration, 5%): a separate change.
