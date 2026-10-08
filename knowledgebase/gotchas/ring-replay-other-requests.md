# A ring buffer bundle replays only startup and the failing request

Status: Observed (2026-10-08, testdata/replay/server)

Symptom: `xo replay` of a crash bundle written with `XO_RING=N` (no
`--record`) ended with "the trace ends before main (request 0) calls
http.serve(1) ... without reproducing fault: index 6 out of range", and
the replayed fault said index 5.

Cause: ring buffers keep the effects of main (request 0) and of the
failing request only, with no scheduler decisions. The replay runs main's
startup, delivers the failing request alone, and stops. The handler read
a counter that an earlier request had incremented, which the bundle does
not hold, so the fault differed.

Fix: none needed in the runtime; it is the documented limit of ring
buffers. A fault that depends on state built by other requests (or on an
interleaving) needs a full recording (`xo run --record` or `XO_RECORD`).
The test route `/boom` faults on the request's own path length.
