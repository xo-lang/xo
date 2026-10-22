# A worker's blocking network poll fired a timer, then waited forever

Status: Observed (2026-10-21)

## Symptom

With decision 0324's runtime (idle workers poll the network), an xo
built from it hung while building compiler/xo: 0% CPU, a defunct child
process never reaped, every worker in its futex wait but one, which sat
in kevent with no timeout holding the poll token, and the poller thread
asleep waiting for that worker to give the token back.

## Cause

poll.xo poll_round fires the due timers before it waits for events. A
worker's poll pushes the fibers it readies onto its own ring with no wake
each (W_NONEXT 2, pool.xo np_poll); the rule "do not wait when a timer
fired" was written for the non-blocking poll (maxwait >= 0) only. An idle
worker's blocking poll (maxwait -1) fired a due timer, which put the
sleeping fiber on that worker's own ring, and then blocked in kevent with
no other timer left: nothing could end the wait, and no other worker
looks at that ring while the worker is off the idle list. The fiber was
proc.xo wait_exit's waitpid loop (a sleep between WNOHANG checks), so the
child's exit was never collected.

## Fix

poll_round takes `worker: Bool`; a worker's poll never waits after
firing a timer, blocking or not (the poller thread's pushes go through
the inject queue and a wake, so it may still wait). Golden
rt_poll_timer (one task sleeping 10 us 3000 times) hung 5 of 5 runs
before the fix and passes after.

Rule: any poll that readies fibers onto the polling thread's own queue
without a wake must not block afterwards in the same round.
