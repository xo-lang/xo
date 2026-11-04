# A driver's wire protocol change deadlocks tests with a fake server

Status: Observed (2026-11-03)

**Symptom.** `xo test --std-module=native/llrt/pg` failed "a fake server: rows decoded by
type" with `XO-F001: deadlock: every task is blocked and no timer is pending` (main red in
hourly 2244).

**Cause.** 9573b3879 made `PgConn.query` send Parse/Bind/Describe/Execute/Sync for every
statement; the test's fake server answers only the simple query message `Q`, so it sent
nothing back and the client waited on a read no task could finish. A fake server that
speaks a subset of a protocol turns a protocol change into a deadlock, not an error.

**Fix.** A statement without arguments keeps the simple protocol (01bd93f7a); the change
to a driver's messages runs that driver's own `--std-module` test, not only the corpus
(the sql corpus uses its own in-memory driver and passed).
