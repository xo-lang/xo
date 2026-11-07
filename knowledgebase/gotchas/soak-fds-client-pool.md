# A soak's open fds grow with the load client's pool, not a leak

Status: Observed (2026-11-07, Xo Lang 2)

Symptom: a `harness soak` run reports `ALERT: open fds grow` and DRIFT: the
app's fds climb in steps over hours (28, 78, 85, 108, 150 in a 6 h soak of
stability/notes at 1d8571422; 202 at 7266bf502), sockets account for the
growth, and the `client sockets` line equals the app's sockets.

Cause: the load client's own connection pool. stability/harness load.go's
newDriver keeps up to 256 connections per host (poolCap), and before
2026-11-07 the soak's `-client-idle` defaulted to 0, so a pooled connection
was never closed. Each latency spike puts more requests in flight, the
client dials more connections, and both ends keep them for the rest of the
run. The fd count is the run's high-water mark of concurrency, which ratchets
up at each spike and is bounded by poolCap. A run with worse tail latency
shows more fds (the 0533 soak's 202 matched its higher p99), which looks
like a leak in the change under test.

Fix: the soak now defaults to `-client-idle 30s`, so idle connections close
and fds follow concurrency, and the drift check judges the fds the client
does not hold (the app's fds minus the client's sockets) and the client's
sockets against poolCap, printing a note instead of an ALERT for pool
growth. To compare with a soak from before 2026-11-07, run with
`-client-idle 0` (the harness is built at the soaked revision, so a soak of
an old revision keeps its old default). Before chasing an fd ALERT as a
server leak, compare `client sockets` with the app's sockets and read
`client dials`: if dials step with the sockets, the client opened them.
