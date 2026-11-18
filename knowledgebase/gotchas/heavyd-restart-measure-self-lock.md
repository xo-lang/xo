# heavyd restart: a draining measurement waits on itself

Status: Observed (2026-11-18)

Symptom: after a heavyd restart (05:36, b76df81b0) no Mac run started for
24 minutes. Gates waited on "a measurement drains", the measurement
l2-llvmab2 waited on "a measurement runs" and then "a gate has waited past
HEAVY_GATE_YIELD_MIN", and bulk runs waited on the queued measurements.

Cause: the measurement held locks/measure and locks/draining under the old
daemon. Its heavy.sh client (the PID in both files) reconnected with its
ticket and was put back in the measure queue, but the files stayed: alive
PID, so tidy kept them. It waited on its own lock, and the gates on its
drain.

Fix: enqueue removes the measure lock and the draining mark a reconnecting
measurement holds (scripts/lead/heavyd/daemon.go;
TestMeasureReconnectReleasesItsLocks). Lock files keyed by a client PID
must be dropped when that client re-enters a queue, not only when it dies.
