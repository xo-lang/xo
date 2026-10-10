# Woken Mutex readers park again behind a queued writer

Status: Observed (2026-10-10, decision 0118)

Symptom: `bench/perf/conc` `rwmutex` (64 tasks, a quarter of the
accesses `with`, the rest `with_read`) took 967 ms and 6.3 s of CPU
natively against 11 ms in Go; `XO_RTSTATS=1` showed 912,000 fiber parks
and 532,000 slow lock acquisitions for 256,000 accesses.

Cause: a writer's release woke every waiting reader, and each checked
again when it ran. Writers are preferred, so with many tasks a writer
had almost always queued by then (`wwait > 0`), and every reader parked
again: about 3.6 parks per access, each with a trip through the Mutex's
lock.

Fix: the release hands the lock to the waiting readers
(`mutex_grant_readers`: counted, unlinked, marked granted before the
wake up), so a woken reader returns at once. Spinning before parking
does not help here (measured, decision 0118). To see such convoys, run
the workload with `XO_RTSTATS=1` and compare `fiber_park` with the
number of operations.
