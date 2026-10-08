# Record and replay: every decision and every outside event is logged

Status: Observed (2026-10-08, record and replay, core 11.3)

Symptom (avoided by design, then confirmed by the interleave tests): a
replay that uses the same seed as the recording takes another
interleaving, or hands the turn to a thread the trace never picked at
that point.

Cause: under the real clock the ready list depends on real time (I/O
completing, timers firing, requests arriving). The seeded PRNG chooses an
index into that list, so the same seed picks another thread when the list
differs. `sched.yield` also skipped the decision when no other thread was
ready, and in a replay a thread back from (instant) I/O may already be
ready there.

Fix (std/rt/replay.go, sched.go):

- A recording logs every pick by thread id (run length encoded), and
  `yield` always decides under record and replay, even with an empty
  ready list.
- Timers (`realAfter`), signals, request arrivals, and server ends are
  events: posted to the scheduler's inbox and run at the next decision
  while no thread holds the turn, logged in order with the picks. A replay
  fires them at the same point; replay timers never use real time.
- Thread ids, timer ids, and request ids are given only by the running
  thread or inside an event, so they match between the two runs.
- A replay waits until the thread the trace picks is ready; a watchdog
  reports a divergence after 3 s without progress.
