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
about 71k to 107k req/s (decision 0044). To find this class of stall,
build the server, run it with `XO_BLOCKPROFILE=<file>` (and
`XO_CPUPROFILE`), load it, stop it with `kill <its PID>`, and read
`go tool pprof -top -cum <binary> <file>`.
