# A reader that waits for another reader deadlocks behind a queued writer

Status: Observed (2026-10-08, std/rt/rwlock_test.go, seed 8)

Symptom: a test with two `Mutex.with_read` sections that hand a value to
each other over a channel, plus one `with` writer, hung under the
deterministic scheduler for some seeds (seed 8 of 20) and finished for
the others. The report:

```
task 2 blocked in receive on a channel that no task can send on
task 3 blocked in Mutex.with_read on a mutex, held for reading by task 2; writers waiting: task 4
task 4 blocked in Mutex.with on a mutex, held for reading by task 2; writers waiting: task 4
```

Cause: writer preference (decision 0063). Reader 2 entered and waited for
reader 3; the writer queued before reader 3 arrived, so reader 3 queued
behind the writer, and the writer waits for reader 2. Go's
`sync.RWMutex` has the same rule ("no reader may wait for another reader
to enter"). The program is wrong, not the lock: only the interleaving
decides whether it shows.

Fix: never block inside `with_read` on something only another reader of
the same mutex can provide; take what you need, leave the section, then
coordinate. To find such code, run the tests with `xo test --explore=N`:
the deterministic scheduler reaches the bad order and reports XO-F001
with the readers holding the mutex. The rt test pins this
(`TestSchedReaderWaitsForReader`).
