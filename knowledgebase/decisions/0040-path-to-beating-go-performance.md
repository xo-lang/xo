# 0040: Path to matching or beating Go on performance

Status: Proposed (2026-10-08). Pending experiments in work queue items 4 and 5.

## Context
The user asked whether Xo could be faster than Go. P1 compiles to Go, so Go's
speed is the ceiling today, minus Xo's overheads: checked arithmetic (about
1.4x on a tight loop), closures in higher order code (about 8x on `map`/`fold`),
and persistent collections (Map updates now 3.2 to 4.4x after
`bench/results/perf-map-2026-10-08.md`, from 45x).

## Analysis
Xo's guarantees give a compiler information Go's does not have:

| Xo guarantee | Optimization it enables | Go's handicap |
|---|---|---|
| No mutable aliasing (value semantics) | in place reuse of unique values, stack allocation, stronger optimization | pointers alias, so the compiler is conservative |
| Structured concurrency | per request arenas freed at scope exit with no GC tracing | GC is a top backend cost; Go's arena experiment was abandoned |
| Effects in signatures | memoize, deduplicate, parallelize, constant fold pure functions | purity is unknown |
| Monomorphized generics | specialized code, no runtime dictionaries | GC shape stenciling makes generic code slower than hand written |
| Sum types, no nil | no nil checks, enum dispatch without interface boxing | interfaces box values |
| Own backend (P3, for example LLVM) | full optimizer | Go's compiler optimizes lightly, typically 10 to 30 percent behind on compute |

Precedent: Koka and Lean 4 use reference counting with reuse of unique values
("Perceus") and run functional, immutable by default code close to C.

Where Go stays ahead: a mature scheduler and network poller, a decade of
runtime tuning, and no checked arithmetic.

## Proposed position
P1 will not beat Go. A P2/P3 Xo with arenas, unique value reuse, and its own
backend could plausibly beat Go on allocation heavy request handling and on
generic or pure compute, and roughly match it elsewhere. This is a hypothesis
to measure, not a promise.

## Experiments (work queue)
4. P1 monomorphization: emit specialized Go per generic instantiation instead
   of Go generics; benchmark generic heavy code against idiomatic Go generics.
5. Per request arena prototype in the runtime; compare GC pauses and
   throughput with Go on task 01 under load.

Accept, amend, or reject this record based on those results.

## Evidence
- Monomorphization (work queue item 4, `bench/results/perf-mono-2026-10-08.md`,
  reproduced independently): with `xo build --mono`, Xo against idiomatic Go
  generics is 0.49x on `maxby` (twice as fast), 0.89 to 0.92x on `hof`, 0.96x
  on `sortpt`, 1.14x on `sortint`, and 2.26x on `iface`. The remaining losses
  are checked Int arithmetic (iface runs in 6 ms against Go's 15 ms with
  checks stubbed) and O(n) `List.insert`. First measured case where P1 Xo
  beats Go.

