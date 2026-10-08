# A 4 MB heap on 12 Ps collects after far fewer bytes than it should

Status: Observed (2026-10-08, bench/perf/http, Apple M-series, 12 P; measured below)

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

## Measured, 2026-10-08 (Go 1.26.1)

`GODEBUG=gctrace=1,gcpacertrace=1` on both servers under run.sh load
(12 P, default GOGC): both run at the 4 MB minimum goal with the same
live heap (`/gc/heap/live:bytes` mean 1.54 MB Go, 1.49 MB Xo
`--no-arenas`; with arenas 1.97 MB and a 5 MB goal), no forced GCs, no
finalizers or `runtime.GC` calls in std/rt. So the goal is not smaller
for Xo. What differs is how much of the headroom real allocation gets.
Per cycle the pacer's heap grows from the marked heap to the mark end
(`in T B -> E B`): Go 1.54 to 3.84 MB, Xo 1.49 to 3.76 MB, about 2.3 MB
for both; real allocation per cycle (`/gc/heap/allocs:bytes` / cycles)
was 1.11 MB for Go and 0.51 MB for Xo. The rest, 1.2 MB (Go) and 1.8 MB
(Xo) per cycle, is bytes the pacer counted that no object used. It grows
with GOMAXPROCS (headroom minus real allocation per cycle, P = 1 to 12:
Go 0.37 to 1.35 MB, Xo 0.44 to 2.0 MB, about 0.09 and 0.14 MB per P),
which is what span slack predicts: `mcache.refill` adds a span's free
bytes to the pacer's heap when a P takes it, and only the flush at the
next GC takes back what was not used. At a GC every 1.3 ms each P
allocates a few objects per span class per cycle, so most of each cached
span is counted and unused. Xo touches more span classes per request
(54 size classes with any allocation against Go's 43, including the 576
byte request block and, without arenas, 1.5 to 8 KiB pool refills from
frequent-gc-empties-pools.md). Xo also triggers earlier (mean trigger
3.33 MB against 3.67 MB: cons/mark 0.33 against 0.13), a smaller effect.

The cost is per cycle, not per byte: at GOGC=800 (a 32 MB minimum goal)
GCs per request follow bytes per request (Go 1.03, Xo 1.25 and 1.58 per
10k against 3.0, 3.5, and 4.7 KB per request). A micro benchmark that
spread garbage over 1 to 32 size classes from 12 goroutines did not
reproduce a class count effect (2.4 to 2.6 MB per cycle), so the class
count part is consistent with the data but not isolated; the P
dependence and the per cycle constant are measured. Decision 0065 raises
the goal for Xo programs.
