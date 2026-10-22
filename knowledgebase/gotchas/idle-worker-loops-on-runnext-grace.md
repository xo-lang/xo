# Idle workers loop on a run-next slot they may not take yet

Status: Observed (2026-10-22, HTTP/2 256 MiB download server, linux/arm64)

Symptom: the HTTP/2 download server used 2 to 2.6 times Go's CPU on
Linux at the same rate. The CPU profile showed 35% in `pool.find_work`,
13% in `sysio.mono_ns` (vDSO `clock_gettime`, unnamed frames) and 11% in
`epoll_wait`, all under idle workers. `XO_RTSTATS=1` over three
downloads: `spin_round=4113167`, `idle_npblock=2662065` and
`poll_return=598056`, against `fiber_park=7589` and `wake_push=7461`.
Reading the clock once per steal round instead of once per victim
(and skipping it in a poll with no timers) measured within noise.

Cause: the connection's fiber hands work to the next one through its
worker's run-next slot, which a thief may take only after
`NEXT_GRACE_NS` (50 us). A woken worker found the slot in its grace,
could not steal it, and went idle; `np_block` (and `go_idle`'s check
after listing itself) asked `any_work`, which counted any set run-next
slot as work, so the worker returned at once to look again: millions of
empty steal rounds and nonblocking polls, each with a clock read.

Fix: `any_work` returns 2 when the only work is run-next slots still in
their grace. `np_block` then waits in the poller for at most the grace
(epoll rounds it up to 1 ms; kevent keeps the nanoseconds), and
`go_idle` waits on its futex for the grace (or a wakep), then looks
again and steals the slot if its owner still has not taken it. Server
CPU 0.664x (`xo bench --ab`, Docker linux/arm64), the rate equal to Go's
(bench/results/h2-idle-loop-2026-10-22.md).

How to avoid: a "may this worker sleep" check must agree with what the
worker may take. Count the rounds (`XO_RTSTATS` spin_round,
idle_npblock, poll_return against fiber_park) before tuning the cost of
one round.
