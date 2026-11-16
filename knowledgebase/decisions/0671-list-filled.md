# 0671: List[T].filled(n, x), a list of n copies of x made in bulk

Status: Proposed (Xo Lang 3, 2026-11-15; stub, filled in by the
implementing push)

## Context

A list of n zeros (or any one value) is built with a push per element.
std/brotli's encoder spends 64% of a quality 6 encode doing that for its
4 MB ring buffer and its hash tables (bench/perf/brotli enc6, Mac
XO_PROF=cpu, 2026-11-15), where Go's make([]T, n) allocates zeroed
memory in bulk.

## Decision

To be filled in: `List[T].filled(n: Int, x: T) -> List[T]`, lowered on
every backend (a mem_set for zero and byte-pattern values, a store loop
otherwise), with an instruction-count guard.

## Consequences

To be filled in.

## Alternatives

To be filled in.
