# A 4 MB heap on 12 Ps collects after far fewer bytes than it should

Status: Observed (2026-10-08, bench/perf/http, Apple M-series, 12 P)

Symptom: after the 2026-10-08 allocation cuts the Xo server (no arenas)
allocated 1.2 times as many objects and 1.75 times as many bytes per
request as the Go server under the run.sh load (37.5 against 30.8
allocations, 5.3 against 3.0 KB), yet ran 4.2 times as many GCs per
request (98 against 23 per 10k). Allocation per GC cycle: Xo 0.54 MB,
Go 1.31 MB, both with a 1 MB live heap and the 4 MB minimum goal
(gctrace `3->3->1 MB, 4 MB goal`, no forced GCs). With `GOMAXPROCS=4`
the same Xo binary allocated 2.34 MB per cycle (Go 2.65 MB) and served
about twice the requests.

Cause (likely, not proven from runtime counters): the pacer counts a
span as allocated when a P's mcache takes it, whole, not object by
object. Each cycle every P that allocates refills one span per size
class it uses, so with a 4 MB goal, 12 Ps, and the many size classes
of the Xo runtime (map tables, log arrays, request blocks, JSON
buffers, and net/http's own), a large part of the 3 MB of headroom is
span slack, and the trigger arrives after a fraction of the real
allocation. Fewer Ps, fewer size classes, or a bigger goal each widen
the real headroom.

Fix / how to measure: do not read GC pressure from bytes per request
alone; compare total_alloc / num_gc (`XO_MEMSTATS`, `MEMSTATS`,
bench/perf/http/allocs.sh and the loadstats line in
bench/results/perf-throughput-2026-10-08.md). The remaining lever is a
larger heap goal for servers (`debug.SetMemoryLimit` with a higher
GOGC, or a minimum goal), which changes memory use and needs a decision.
