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
- Mono default and check elimination (2026-10-08, update section in
  `bench/results/perf-mono-2026-10-08.md`): range analysis removes provably
  safe overflow checks and the remaining checks inline. Against Go: iface
  0.80x, iter 0.32x, sortpt 0.87x, hof 0.83x, maxby 0.49x, sortint 1.08x
  (List.insert is O(n)). Overflow still faults with values and position.
- Arenas (2026-10-08, `bench/results/perf-arenas-2026-10-08.md`): task 01
  under loopback load at 32 workers. Go 123k req/s, p99 872us, GC pause
  2.9 ms per 10k requests. Xo 52k, 993us, 17.3 ms. Xo `--arenas` 73k, 834us,
  4.8 ms. Arenas cut Xo's GC cost 3.6x and beat Go's p99, but Xo allocates
  about 2x Go per request, so Go keeps higher throughput. The claim "beat Go on
  allocation heavy request handling" is not supported in P1 yet; the next lever
  is fewer allocations per request (unique value reuse, queue item 9h).
- LLVM backend prototype (2026-10-08, `bench/results/llvm-prototype-2026-10-08.md`),
  ms, Go / Xo via Go backend / Xo via LLVM -O2: fib 225/224/231, sieve
  111/>60000/113, nbody 162/18760/183, sort 267/>60000/232, interp
  144/583/67, collatz 491/601/490. LLVM Xo is at or near Go on compute and
  beats it on sort and the interpreter loop (a hand tuned Go interpreter runs
  in 30 ms), with a bump allocator that never frees (up to 2.9x Go's memory).
  The Go backend's catastrophic cases are O(n) list element writes, a bug, not
  a language limit.
- List writes (2026-10-08, `bench/results/perf-list-2026-10-08.md`): the Go
  backend now writes unshared lists in place (sticky shared flag set at copy
  points). ms against Go: sieve >60000 -> 149 (110), nbody 18632 -> 256 (162),
  sort >60000 -> 274 (266), interp 588 -> 173 (143).

