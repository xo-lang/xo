# A soak run outside the heavyd lanes overlaps exclusive jobs

Status: Observed (2026-11-06, Xo Lang 2's 0533 after-soak on offload host 2)

**Symptom.** A 6 hour `harness soak` of stability/notes ended DRIFT in its
last hour: p99 jumped from about 2 ms to 291 ms (p999 1.48 s) in one
sample, the app's open sockets stepped from 95 to 204 in the same sample
and stayed there, and CPU per request rose. Nothing in the server had
changed at that time.

**Cause.** Long soaks run with OFF_NO_LANE=1 (offload.md: past 30 minutes,
outside the lanes), so heavyd does not see them, and an exclusive job
(another agent's `json32-ab.sh` at 19:53) took host 2 about 7 minutes before
the jump. The starved load client's requests outran its pool's reuse, so
it dialed new connections, which the app then held as idle keep-alive
connections: the socket step was the client's, not a leak.

**Fix.** Read a soak's late rows against the host's other jobs before
blaming the change: log `heavy.sh --status` lanes each hour of the soak
(Xo Lang 2's base6h-lanes.log), and use the harness's client_dials column
(samples.csv) and "client dials" report line: a socket step matched by as
many dials is the client reconnecting. heavyd is being changed so an
exclusive job no longer starts beside a no-lane run (lead, 2026-11-06).
