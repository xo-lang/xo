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
