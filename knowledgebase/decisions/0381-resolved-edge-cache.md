# 0381: A program's resolved edge list is split, and the std graph is cached once for the std modules a program loads

Status: Proposed (Xo DeepSeek 2, 2026-10-26, for the lead; nothing built).

## Context

`xo dev edges` now checks the std implementation files' bodies (their
sites are most of what a corpus calls; the check was 3,222 edge lines
before that and 251,718 after on one small corpus). The preloaded check
is 16 s for a small corpus against 0.3 s for `xo check`, so the full
890-program sweep is 3.5 to 4 hours, which is too slow to run routinely
and too slow to sit in a gate. Measured 2026-10-26: checking without a
preload takes 0.3 s; preloading only the impl modules of the std modules
a program actually loaded (a two-phase check) is 12 to 14 s and yields an
identical covered set on five programs (1,390 uncovered and 223
over-claims in both, only the edge count differing), so the second check
saves 20% and would be a permanent complication. The cost is the llrt
runtime's bodies, which the checker already reads for `xo refs`.

## Decision (proposed)

1. Split the edge list in two: the program's own edges (sites in non-std
   files) and the std graph (sites in std files) for the exact set of std
   modules the program's check loaded. The second is a function of that
   set, xo's identity and the flags, so it is cached under the cache dir
   as a new kind ("edges"), keyed the way the backend caches key (xo
   identity, the std sources' hashes, the module set, the cache version).
2. `xo dev edges` reads the std graph from the cache and checks only the
   program, so a repeat sweep is dominated by the program's own check:
   about 0.3 s a program, an 890-program sweep in minutes. A miss costs
   one full check and fills the entry.
3. `XO_EDGES_CACHE=off` disables the cache; `XO_BACKEND_CACHE=verify`
   re-derives and compares, for the gate that would use it. The cache
   version is part of the key, as the arm64 function and part caches do
   it.
4. The output format does not change (SRC_PATH SRC_LINE SRC_NAME
   TGT_PATH TGT_LINE TGT_NAME SITE_PATH SITE_LINE SITE_COL SITE_NAME), so
   internal/stdcoverage's comparison reads a cached sweep unchanged.

## Consequences

A full sweep becomes routine, which makes the resolved comparison usable
as a gate and makes the scan-vs-edges numbers checkable every release
instead of as a one-off. The cache needs a format with a version and a
verify path: a stale entry would quietly under-report coverage. A
program whose std usage changes misses on the module set (the entry is
per set, which is coarser than per program, so a release's 890 programs
share a handful of entries, not 890 files of 4 MB each). The first sweep
after a std change pays one full check a distinct module set.

## Alternatives

- A per-program cache keyed on the program's own files, flags and xo
  identity: simplest, but every entry holds the whole std graph (about
  4 MB) so a release's sweep stores gigabytes, and every program misses
  after a std change.
- The two-phase check alone: 20% for a second check a program and two
  code paths, measured and dropped.
- Decision 0380 (a std module's checked interface cached) is a build-side
  cache of checked modules, not of resolved edges: it is complementary,
  and if it lands the impl-graph check gets cheaper as well. Nothing in
  it caches the edge list or the graph.
