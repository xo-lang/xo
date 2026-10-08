# Closed loop load makes latency look better on a slower server

Status: Observed (2026-10-08, bench/perf/http)

Symptom: at 32 workers Xo with `--arenas` reported a lower p99 (834 us)
than Go (872 us) while serving 0.6x Go's requests per second. Read
naively, Xo had the better tail.

Cause: `bench/perf/http/loadgen` is closed loop: each worker sends its next
request only after the previous response. A server that completes fewer
requests per second has fewer requests queued per connection and per
core, so its latency distribution is not comparable to a faster server's
at the same worker count. Only latency at equal throughput (or an open
loop, fixed arrival rate generator) compares two servers.

Fix: compare latency between builds of the same server (Xo against Xo
`--arenas`), where the comparison is fair, and report Go's latency next to
its throughput rather than as a ranking. At 8 workers, where both are
below saturation, Go's p99 is lower (274 us against 324 us). For a real
latency comparison, add a fixed rate mode to the load generator.
