# bench/perf/tls/measure.sh runs in two sessions at once fail certificate checks

Status: Observed (Xo Lang 2, 2026-10-17).

## Symptom

Every workload of `bench/perf/tls/measure.sh` printed
`error: Protocol(msg: "... tls: failed to verify certificate: x509:
certificate signed by unknown authority (possibly because of \"x509:
ECDSA verification failure\" ... \"bench tls CA\")")`. The next run of
the same compiler a few minutes later passed, and a bisect over the
commits in between passed at every one. It looked like a miscompile and
was not one.

## Cause

measure.sh's servers listen on PORT (default 18120) and the 10 ports
after it, with keys made fresh for each run. While another session ran
measure.sh on the same default ports, this run's server could not bind,
and its client reached the other session's server, which has another CA.

## Fix

Give each session its own ports: `PORT=19420 sh bench/perf/tls/measure.sh
...` (any base no other session uses, 11 ports wide). Treat a
certificate failure in a measurement as a port clash first, not a
compiler bug.
