# Spinning before parking on the runtime lock made throughput collapse

Status: Observed (2026-10-08, bench/perf/http task 01, 32 workers)

Symptom: adding a spin phase to `(*lock).acquire` (std/rt/sync.go)
before a task parks dropped throughput from about 71k to 25k req/s (and
to 11k after a second change), with p99 rising from under 1 ms to 15 to
50 ms.

Cause: two different spins, two failures. `runtime.Gosched()` in the spin
loop puts the goroutine on the global run queue, which processors with
local work poll only every 61 scheduler ticks, so a spinner can sit out
for milliseconds. A timed busy loop (20 us) with 32 contenders keeps
processors busy spinning, and the lock holder or the woken waiter then
waits for a processor. Go's sync.Mutex spins only a few times and only
when few spinners and idle processors exist, which the runtime cannot
see from outside.

Fix: no spin in the runtime lock. Barging plus a `runtime.Gosched` by the
releaser after a wake (decision 0044,
`gotchas/lock-convoy-runnext-wake.md`) fixed the convoy without spinning.
