# A FIFO handoff lock convoys: the woken goroutine waits in runnext

Status: Observed (2026-10-08, bench/perf/http task 01, 32 workers)

Symptom: the Xo server used only about 3.3 of 12 cores and Xo code was
about 5 percent of CPU samples, yet throughput stalled near 70k req/s.
The block profile (`XO_BLOCKPROFILE`) put 96 percent of blocked time in
`(*lock).acquire` under `Mutex_with`. Instrumented, the store mutex was
held 2.2 us per acquisition (lock busy about 14 percent) while the
average acquire waited 375 us.

Cause: release handed the permit to the first waiter and closed its
channel. Go readies a goroutine woken by a channel close into the
waker's runnext slot, so it runs only when the waker blocks (for an HTTP
handler: after writing the response and parking on the next read), or
when another processor steals it after a delay (runqgrab sleeps a few us
before stealing runnext; `runtime.usleep` was 20 percent of CPU samples).
Meanwhile the permit is owned but unused, so every other request queues
behind it. Barging alone (free the permit, wake the waiter) did not help:
almost every contender was already parked, so the woken one still had to
run before anyone took the lock.

Fix: after waking a waiter in a real run, the releaser calls
`runtime.Gosched()`, which lets the woken goroutine run on this
processor at once; combined with barging this took the benchmark from
about 71k to 107k req/s (decision 0047). To find this class of stall,
build the server, run it with `XO_BLOCKPROFILE=<file>` (and
`XO_CPUPROFILE`), load it, stop it with `kill <its PID>`, and read
`go tool pprof -top -cum <binary> <file>`.

Update 2026-10-09 (Observed, native backends): the same convoy exists in
the native scheduler (decision 0077): a fiber woken by a Mutex release
went to the releasing thread's `next`, which thieves leave alone for 50
us (XO_NEXT_GRACE_NS), while the releaser finished its HTTP request.
Fix there: `mutex_signal` puts the woken waiter in the ring, where a
spinning worker takes it (p99 900 to 800 us on bench/perf/http, decision
0082). Channel and task wake ups keep `next` (ping pong hands over on one
thread).

Update 2026-10-22 (Observed, native backends): the ring did not end the
convoy for writers. A release woke the next waiting writer into the ring;
until it ran, the Mutex was free, yet the waiting writer kept readers
out, so every arriving reader parked (CRUD: about 0.9 Mutex parks per
request on 1 core as on 6; a micro test, one park per operation on 4
threads and none on 1). Spinning never helped (Go's spin rule refuses
while the worker has queued work), and handing the Mutex to the woken
writer did not either (readers still queue behind its wake). Fix: the
release puts the woken writer in next and yields to the back of its own
ring, the native form of the Gosched fix above, only on a Mutex that
with_read has taken (decision 0118's 2026-10-22 addendum: CRUD 2.15x and
1.93x at 3 and 6 cores). To find it: XO_RTSTATS=1 sig_mutex per request
near 1 at any core count, and an execution trace (XO_TRACE) whose parks
by reason are mostly `mutex`.
Correction (2026-10-22 08:40): the yield's gain is not the convoy's end.
Readers parked behind a free Mutex and the Mutex parks stay as many with
the yield; what falls is workers sleeping and being woken to take the
woken writer (futex_wait 1.18 to 0.25 per request at 6 cores; decision
0118's correction). Count before naming a mechanism: an XO_RTSTATS probe
of the event the explanation needs is a few lines.
