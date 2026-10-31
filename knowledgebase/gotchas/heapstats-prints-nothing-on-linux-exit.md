# XO_HEAPSTATS printed no report on a normal exit on linux/arm64

Status: Anticipated (2026-10-31; seen once, cause not known)

Symptom: a server built with `xo build -o srv bench/perf/idleconn/xo` and
run as `PORT=18050 XO_HEAPSTATS=1 STOP_AFTER_MS=25000 ./srv` on the
linux/arm64 offload host, with 2,000 idle connections held (plain and TLS),
ended through its normal exit (the listener was closed, so `serve()`
returned an error and `main` returned) and printed only the program's own
error line: no XO_HEAPSTATS table (blocks on each class's free list,
chunks carved). `os.proc.exit(0)` printed nothing either. The report is
documented (infrastructure/memory-footprint.md "Measuring this").

To reproduce: `bench/perf/idleconn/xo` at the commit that added
STOP_AFTER_MS (tlssurvey branch), `STOP_AFTER_MS=25000 XO_HEAPSTATS=1`,
the client `bench/perf/idleconn/client -n 2000 -tls` (see the heapstats job
in notes/lang3). Compare with a tiny program that allocates and returns.

Why it matters: with the heap profiler missing TLS allocations
([heap-profile-misses-tls-allocations.md](heap-profile-misses-tls-allocations.md))
there is no tool that names the size class holding the 12 KB a TLS
connection costs over a plain one (24 KB against 10.6 KB at 2,000
connections; 18.8 KB against 6.4 KB at 40,000). Fix the tools, then take
that lever (decision 0449, TLS memory).
