# 0381: A program's resolved edge list is split, and the std graph is cached once instead of checked with every program

Status: Accepted (owner, 2026-10-30); built 2026-10-30 (xo-lang-6, see As built). Previously proposed (Xo DeepSeek 2, 2026-10-26, for the lead; nothing built).

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
routinely instead of as a one-off. The cache needs a format with a version
and a verify path: a stale entry would quietly under-report coverage. One
entry holds the std graph, about 22 MB of text (245,466 rows), not one per
program and not one per module set, so a release stores a handful of
entries at most; the first sweep after a std change pays one full check.

The audit path (decided 2026-10-26, lead, after the 890-program
comparison ran as a one-off): the `-short` coverage check stays the name
scan, because it costs seconds where the sweep cost 3.5 hours and 22 GB,
and because its error direction is the safe one (over that run, of 1455
declarations the scan called 126 uncovered, the edges called 187, and the
scan never missed one the edges reached). The resolved comparison is a
periodic audit instead: its procedure, its cost, and the last run's
numbers and reasons live in knowledgebase/infrastructure/agent-tools.md
under "std coverage", and its product is the list of declarations the scan
credits that no edge reaches, with a reason each (interface dispatch
against a corpus gap). Those entries are not recorded in
internal/stdcoverage/allow.txt, which means "uncovered and accepted".

## As built (2026-10-30, xo-lang-6)

- compiler/cli/servers edges.xo `edges_of` checks the program and
  returns its edges split: the std graph is every edge whose site is in
  a file of the std tree (check.Module std_tree: std, native, llrt), and
  with --impls every implementation edge with both ends there; the
  program's own is the rest (its files, a binding's declaration file, an
  impl edge with a program type at one end). It also reports the
  bindings the program's modules use (`go`, `c`, both or none).
- compiler/xo edgecache.xo holds `xo dev edges`: it checks the program
  alone first (no std bodies, about 0.3 s), which gives its own edges and
  its bindings, then reads the std graph from
  `<backend cache>/edges/<key>`; the key is the SHA-256 of the cache
  version, xo's identity (bcache.xo self_id, the executable's hash),
  the bindings, --impls, and every std source's path and content hash
  (not sizes and times as the std cache keys: a worktree at the same
  commit shares the entry). The output is the program's own edges plus
  the entry, sorted, the same lines in the same order as the uncached
  command. A miss checks again with the std bodies, stores the std part
  (written under a temporary name and renamed), and prints its own
  edges and std's from that check; when the program's own edges differ
  between the two checks it says so on standard error (and fails under
  verify), which is the one way a cached graph could misreport a
  program. A program in the std tree (no own edges) takes the uncached
  path. `XO_EDGES_CACHE=off` or `XO_BACKEND_CACHE=off`: the one check
  with the std bodies, no cache; `XO_BACKEND_CACHE=verify`: both checks,
  the std graph compared with the entry and the own edges between the
  checks, exit 1 on a difference. `XO_CACHE_STATS=1` prints
  `cache edges off|miss|hit|verify`. Entries are marked used on a hit
  and pruned with the backend cache (store-log kind `edges`).
- The bindings flag covers `use c` too: a `use c` program's check also
  takes the binding path, and the sample held none to show whether its
  std graph is the plain one, so it gets its own entry.
- Tests: cmd/xo TestDevEdgesCache (strwhere: off, miss, hit and verify
  print identical bytes, one entry stored, no difference reported);
  the existing TestDevEdges and the missing binding tests; the new
  XO_EDGES_CACHE classified in internal/xotool codegen_test.go.
- Measured (linux/arm64 offload host, 2026-10-30, xo at ee57ab0e8 with a private
  backend cache: over ten programs (bench/perf base64, json, alloc,
  http, big; the json_decode_fields, json_depth, json_omitempty,
  codex_gaps and strwhere goldens) the output with the cache off, on the
  miss, on the hit and under verify is byte for byte the same (291,039
  to 291,719 lines each, 290,988 of them the std graph, one 30 MB
  entry); wall per program 18.6 to 22.5 s off, 19.5 to 21.2 s on the
  miss, 142 to 343 ms on the hit, 19.1 to 22.4 s under verify. The
  sweep's first 20 programs took 366 s with the cache off (18.3 s each,
  about 5.3 hours projected for the 1035 program list); every program
  with the cache took 202 s (195 ms each): 1007 hits, 3 full checks (the
  first program fills the entry; two programs in the std tree take the
  uncached path), 25 `use go` or `use c` programs without a binding on
  the host (exit 1 naming it, as before). The `use go` program of the
  2026-10-26 sample (test_go_callback_task, bound first by `xo check`)
  was checked on the Mac (darwin/arm64): the same four ways byte for
  byte (291,273 lines, 161 its own), 19.1 s off against 159 ms on the
  hit; its std graph has 291,028 lines against a plain program's
  290,988 (std/go's bodies), under its own key (bindings `go`), two
  entries in the cache after a plain program follows it.)

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
