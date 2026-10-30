# A deadline moved later before every read woke the poller every read

Status: Observed (2026-10-30, decision 0430's timing, std/native/llrt poll.xo)

Symptom: 100,000 one byte echo round trips with a read deadline set before
each read (Go's SetReadDeadline pattern) ran 1.13x Go on linux/arm64, with
no deadline 0.98x. `XO_RTSTATS=1` showed the difference: about 22,000 more
worker wakes, idle sleeps and futex waits (wake 23,375 against 1,554).
Making the timer once per descriptor instead of once per wait changed
nothing (`--ab` 1.008, within noise).

Cause: a timer that becomes the heap's first pokes the poller (a byte in
its self pipe, or a posted completion on Windows), so it waits less. With
a deadline set before each read the read's timer was always the only
timer, so every wait poked the poller, which woke, found nothing due, and
woke a worker on its way back to sleep.

Fix: a descriptor's deadline timers stay in the heap between waits
(pd_timer, decision 0430), and moving one to a later deadline does not
poke (timer_arm): the poller already sleeps no longer than the old, earlier
deadline and looks at the heap again when it passes. A timer moved earlier,
or added, still pokes. Golden conn_deadline_move covers both directions.
