# A ping pong pair on every worker starved timers and the ring

Status: Observed (2026-10-28, found by decision 0394's direct_fair golden; backported to 0.33.1).

Symptom: with default threads, 24 ping pong pairs of unbuffered channels (more than the cores) and a task
that slept 20 ms: the sleeper woke 350 to 380 ms late in 0.33.0, and 2.3 to 2.8 s late with five times the
rounds (with XO_THREADS=1, one pair: a 20 ms timer fired after 124 s).

Cause: pool.xo `find_work` took the worker's run-next slot, then its ring, and only then the inject queue and the
poller (np_poll: timers, sockets). A wake leaves the woken fiber in the slot, so a pair keeps it full and
the later steps are never reached. Go checks the global queue every 61st round and its sysmon polls the network.

Fix: `find_work` serves the ring, the inject queue and the poller before the slot once a slice (SLICE_NS) at
most (the clock is read every 64th call). Test: golden fair_pairs (fails on 0.33.0, passes with the fix).

Second hole (2026-10-28, found by the release tier's TestGoldenTasks/fair_pairs at XO_THREADS=2): that fairness step
polled only when the ring and the inject queue were both empty, which 24 pairs on 2 workers never are, so a 20 ms
timer still woke 3.3 to 5.3 s late. The poll now comes first in the step (queued fibers join the ring behind the
others, and each worker serves the ring once a slice). XO_THREADS=2 is the case to test; default threads empties
the rings and hid it.
