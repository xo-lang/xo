# A ping pong pair on one worker starved timers and the ring

Status: Observed (2026-10-28, found by decision 0394's direct_fair golden).

Symptom: with `XO_THREADS=1`, two tasks handing off over channels while another task slept 20 ms: the timer fired
after 124 s (633 s with the direct switch). Without a handoff pair it fired on time.

Cause: pool.xo `find_work` took the worker's run-next slot, then its ring, and only then the inject queue and the
poller (np_poll: timers, the network). A wake leaves the woken fiber in the slot, so a pair keeps it full and
the later steps are never reached. Go checks the global queue every 61st round and its sysmon polls the network.

Fix: `find_work` serves the ring, the inject queue and the poller before the slot once a slice (SLICE_NS) at
most (clock read every 64th call, or at once after a direct switch ran out the slice). Test: golden direct_fair.
