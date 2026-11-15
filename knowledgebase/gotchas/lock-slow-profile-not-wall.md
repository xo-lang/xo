# Time in lock_slow in a profile is not wall time

Status: Observed (2026-11-15, the darwin large block cache, branch lang1-cadv, not merged)

Symptom: an `XO_PROF=cpu` profile (2000 Hz) of a cold
gcp aiplatform/apiv1 test build on the Mac charged 2,218 ms to
`lock_slow` under `clock_take`. The cause was the large block cache
(decision 0448) calling `madvise` while it held its lock. Moving the
system call out of the lock cut that profile entry to 64 ms. Yet
`xo bench --ab` on the Mac, run in a quiet window, showed no wall gain
on any program:

- xo's self build: 1.006, n=5;
- the aiplatform build: 0.993 wall and 0.997 CPU, with RSS 1.015 worse;
- json10kx20: 0.992;
- alloc: within noise on every metric.

Cause: the samples in `lock_slow` are threads parked behind the holder
while other threads still have work. They are not time on the build's
critical path: other workers fill the cores, and the holder's
`madvise` costs the same time with or without the lock. A big
`lock_slow` share means contention, not lost wall time.

Fix: none needed. Treat a `lock_slow` (or `ulock_wait`) share as a
lead to measure, never as the expected gain. Decide with
`xo bench --ab` before you push a lock change: the branch was kept and
not pushed, because a lock change with no wall gain adds a race
surface for nothing. See
[lock-wait-is-hold-time-in-disguise.md](lock-wait-is-hold-time-in-disguise.md)
for the reverse case, where a profile understated a hold that did cost
wall time, and
[xo-prof-macos-overstates-ulock-wake.md](xo-prof-macos-overstates-ulock-wake.md).
