# 0065: Xo programs keep a floor under the GC heap goal

Status: Accepted (2026-10-08)

## Context

At default settings the bench/perf/http Xo server trailed Go (0.85x with
arenas, 0.71x without; bench/results/perf-throughput-2026-10-08.md, "Fair
comparison") while matching it at GOGC=800. Both servers run at Go's 4 MB
minimum heap goal with the same live heap (about 1.5 to 2 MB). Measured
on 2026-10-08 (gotcha small-heap-gc-per-p-spans.md): each cycle the pacer
counts span slack of every P that allocated, a constant per cycle that
grows with GOMAXPROCS (about 0.09 MB per P for Go, 0.14 for Xo, which
touches more size classes). On 12 Ps it took 1.2 MB (Go) and 1.8 MB (Xo)
of the 2.5 MB headroom, so Xo collected after 0.5 to 0.7 MB of real
allocation, about every 1.3 ms, which also empties every sync.Pool
(gotcha frequent-gc-empties-pools.md). Waste that could be removed was
removed first (request bodies closed, Content-Length on large bodies);
the per cycle constant cannot be removed from Xo code. Only a larger goal
makes it small against the headroom.

## Decision

`rt.RunMain` keeps the heap goal at or above a floor:

- Default floor 2 MiB per P (`GOMAXPROCS` at start), at least 16 MiB:
  24 MiB on the 12 P laptop. Above the floor growth is GOGC=100, so a
  program whose live heap is past half the floor collects exactly as a Go
  program would.
- Mechanism (std/rt/gc.go): after every GC (a finalizer on a sentinel), the
  runtime reads the marked heap, stacks, and globals from
  `runtime/metrics` and sets the GC percent to the largest value at or
  above the base whose Go goal (`live + (live+stacks+globals)*p/100`, and
  Go's minimum `4 MB*p/100`) does not pass the floor. `debug.SetGCPercent`
  takes the heap lock, no stop the world, at most once per cycle.
- Configurable in the main module's xo.mod (spec core 4.2): `gc min_heap
  SIZE`, `gc percent N|off` (the base), `gc memory_limit SIZE`.
- The environment always wins: `GOGC` set means no tuning at all (Go
  applies it), `GOMEMLIMIT` replaces `memory_limit`, `XO_GC_MIN_HEAP`
  replaces `min_heap` (0 means no floor). No default memory limit: the
  floor bounds the extra memory, so there is nothing to cap.

## Measurements (2026-10-08, 12 P, run.sh load, 8 s, two quiet rounds)

| Server | Floor | Req/s | p99 us | Peak RSS MB | GCs in 10 s |
|---|---|---|---|---|---|
| Go | (4 MB, GOGC=100) | 120,297 / 123,881 | 920 / 879 | 23.0 / 22.4 | 3088 / 2658 |
| Xo arenas | 8 MiB | 129,139 / 129,685 | 848 / 815 | 30.2 | 1118 / 1038 |
| Xo arenas | 16 MiB | 132,911 / 133,604 | 804 / 773 | 38.8 / 38.4 | 388 / 382 |
| Xo arenas | 24 MiB (default) | 133,992 / 134,884 | 760 / 759 | 46.8 / 47.7 | 233 / 235 |
| Xo arenas | 32 MiB | 134,049 / 134,439 | 756 / 767 | 55.9 / 54.0 | 168 / 168 |

A third round ran while another agent's test suite loaded the host and
is not used. Throughput is flat past 16 to 24 MiB; peak RSS grows by
about the floor. The interleaved medians of 5 for the default against Go
are in bench/results/perf-throughput-2026-10-08.md.

## Consequences

- A small Xo server uses about 25 MB more resident memory than the same
  Go server (47 against 23 MB) and collects about a tenth as often; a
  program with a live heap above half the floor (12 MiB here) is
  unaffected. CLIs that allocate less than the floor never collect.
- The floor grows with GOMAXPROCS (128 MiB on 64 Ps), because the slack
  it pays for does; a container that limits memory should set
  `GOMEMLIMIT` or `gc memory_limit` (the limit wins over the floor).
- `xo test` binaries are not tuned (only `RunMain`).
- Benchmarks at "default settings" now compare Go at GOGC=100 with Xo at
  its floor; equal setting comparisons pass `GOGC` to both.

## Alternatives

- `debug.SetGCPercent(400)` (or 800) for every program: same floor for a
  small heap (Go scales its 4 MB minimum by the percent) but 5x (9x) the
  live heap for large heaps, which costs memory exactly where it is
  scarce. Rejected.
- A soft memory limit derived from available memory, with GOGC off: no
  proportional growth, so a live heap near the limit makes the GC run
  continuously (bounded only by Go's 50 percent CPU limiter). Kept as an
  opt in (`gc memory_limit`, `GOMEMLIMIT`).
- A heap ballast (a large never touched allocation): the same floor
  without a per cycle hook, but it counts toward `GOMEMLIMIT`, shows up
  in heap profiles, and Go recommends against it since 1.19.
- Only for `http.serve` programs: the slack is per cycle for any program
  that allocates on many Ps, not only servers, and a floor at start of
  `RunMain` is simpler to explain.
- Remove the slack in Xo: not possible from Xo code (mcache accounting
  is in the Go runtime); fewer size classes per request would shrink it
  a little (the 576 byte request block is one candidate).
