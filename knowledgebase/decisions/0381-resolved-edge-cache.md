# 0381: A program's resolved edge list is split, and the std graph is cached once instead of checked with every program

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
   files) and the std graph (sites in std files). Measured, the second is
   the same for every program, not merely for every module set (see
   Measurement): the preload list is built from the files on disk, so the
   checker always checks the same implementation bodies, and a program's
   types cannot change a std site's target because each body is resolved
   once in its own scope and monomorphization is not part of the edge
   tables. So the std graph is cached as one entry per xo identity, flags
   and std sources, under the cache dir as a new kind ("edges"), keyed the
   way the backend caches key (xo identity, the std sources' hashes, the
   cache version, and the go binding's flag, which changes it: the one
   `use go` program in the sample has a different std graph).
2. `xo dev edges` reads the std graph from the cache and checks only the
   program, so a repeat sweep is dominated by the program's own check:
   about 0.3 s a program, an 890-program sweep in minutes. A miss costs
   one full check and fills the entry.
3. `XO_EDGES_CACHE=off` disables the cache; `XO_BACKEND_CACHE=verify`
   re-derives a program's full edge list and compares it with the cached
   std graph plus the program's own edges, which is what catches a
   std-site edge that the sample did not show to be program independent
   (a program whose call makes a std body resolve differently). The cache
   version is part of the key, as the arm64 function and part caches do
   it.
4. The output format does not change (SRC_PATH SRC_LINE SRC_NAME
   TGT_PATH TGT_LINE TGT_NAME SITE_PATH SITE_LINE SITE_COL SITE_NAME), so
   internal/stdcoverage's comparison reads a cached sweep unchanged.

## Measurement (2026-10-26, darwin/arm64)

- 98 of the 99 programs in the sample (the prelude-only set: all 61
  bench/perf programs, the sampled corpus and golden dirs, the scripts)
  produce the identical std part: 245,466 rows, one md5
  (f5efd296...). Only their own edges differ (52 to 177 rows for the four
  json goldens, 5 for the `use go` one).
- Four goldens that import std/json with different code
  (json_decode_fields, json_depth, json_omitempty, codex_gaps) also
  produce that same std graph, byte for byte, while their own edges are
  52, 60, 137 and 177 rows: the module set does not change the std graph,
  because the preload is not derived from the program's imports.
- The exception in the sample is internal/gen/testdata/golden/
  test_go_callback_task, a `use go` program: its std part is 17,708 rows
  with its own md5 (ca4b2dc7...). The Go binding's check takes its own
  path, so the flag is part of the key.
- Costs: 16 s a program with the full preload, 0.3 s for `xo check`, 12
  to 14 s for the two-phase variant (identical covered set on five
  programs, 1,390 uncovered and 223 over-claims in both), 3.5 to 4 h for
  the 890-program sweep.

## Consequences

A full sweep becomes routine, which makes the resolved comparison usable
as a gate and makes the scan-vs-edges numbers checkable every release
instead of as a one-off. The cache needs a format with a version and a
verify path: a stale entry would quietly under-report coverage. One entry
holds the std graph, about 22 MB of text (245,466 rows), not one per
program and not one per module set, so a release stores a handful of
entries at most; the first sweep after a std change pays one full check.

## Alternatives

- A per-program cache keyed on the program's own files, flags and xo
  identity: simplest, but every entry repeats the whole std graph (about
  22 MB of text) so a release's sweep stores gigabytes, and every program
  misses after a std change.
- A per-module-set cache (this ADR's earlier draft): the measurement could
  not distinguish it from a single entry, since the std graph did not
  change with the program's imports, and one entry is simpler.
- The two-phase check alone: 20% for a second check a program and two
  code paths, measured and dropped.
- Decision 0380 (a std module's checked interface cached) is a build-side
  cache of checked modules, not of resolved edges: it is complementary,
  and if it lands the impl-graph check gets cheaper as well. Nothing in
  it caches the edge list or the graph.
