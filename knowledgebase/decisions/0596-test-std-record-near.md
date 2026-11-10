# 0596: An `xo test --std-module` edit declares std from its record

Status: Proposed (Xo Lang 2, 2026-11-10; the perf page's "test builds use more memory" row)

## Context
An edit of a std module's test (`xo test --std-module=native/M` after changing M) peaks at 202 to
233 MB on linux/arm64 against `go test`'s 106 to 158 MB (bench/results/perf-gaps-2026-11-10.md
section 3). The peak is the compiler's heap, not the test binary (already its own process): about
155 to 161 MB right after check, against 25 to 35 MB for a warm `xo build` of coll. A heap snapshot
at the end of a test edit's check: 130 MB in use, 75 MB of it parse trees.

The difference: a build or run loads std's declarations from their record (decision 0388 stage
1a-ii, "cache stdx record loaded (112 modules)"), and `xo test` does not: the record is set up inside
the module check cache's block, which is build and run only (compiler/xo main.xo
check_program_ov). A prototype that turns the record on for `xo test` (branch l2-testlazy, behind
XO_TEST_STDX=1, the module check cache's skipping kept off) gains nothing in an edit loop: the
record's fingerprint is std's whole tree, so every edit of M misses it ("cache stdx declared (no
records for the fingerprint)", json edit 197 against 199 MB). Joining std's trees lazily for tests
(trees_lazy) changed nothing either (json 211 against 214 MB).

The test variant of std's store already solves the same problem for code (stdcache.xo
std_variant_near): it matches a variant whose tree equals this std's once the from-source set (M and
every std module importing it, std_source_set) is left out on both sides.

## Decision (proposed)
1. `xo test --std-module` loads std's declaration record, keyed as std_variant_near keys the
   store: std's tree without the from-source set, so an edit of M (or an importer) still matches.
   The from-source modules are declared and checked from source, as today; every other std module's
   declarations come from the record.
2. The record for a test variant is written by `xo dev stdcache --test` (and by the first test
   build that misses), next to the variant's store and facts.
3. The module check cache's skipping stays off for tests (a test build checks its tests' modules in
   full, main.xo).

## Measure (before Accepted)
- Peak RSS and the heap after check of a test edit of json, base64 and a leaf module, 3 edits each,
  against main, on linux/arm64 host 1 (`xo bench --ab` with release-vs-go.sh's edit guard). Target:
  json's edit at or under go test's 158 MB.
- Identical test output and binaries against main (scripts/identity on the test binary).

## Consequences
- One more record per std test variant in the backend cache (a few MB each).
- The test path shares the build path's declaration loading, so TestModuleCheckAuto and the std
  records' tests cover it.

## Alternatives
- A per-module record (one entry per std module, stamped alone): reuses more across unrelated std
  edits, but rewrites decision 0388's record format.
- Drop parse trees after check: the trees feed the SSA builder; lazy_parse already skips std bodies.
- Build only the importers the test program reaches: after check the importer set (15 for
  base64, 22 for json) is not the cost (a csv edit, one module from source, holds 148 to 152 MB,
  json's 155 to 168 MB), but at the SSA phase it is (prototype below).

## Prototype (2026-11-10, branch l2-testlazy, unpushed; Mac, json edit, 10 of 10 tests pass)
- check/dump.xo run() splits std's modules into the record's and the from-source set's
  (sep_source); the record and its fingerprint and header cover the first, the set is declared
  after the block; export.xo leaves the set's modules out of the record. Gated by XO_TEST_STDX=1;
  XO_TEST_TREES_LAZY=1 also joins std's trees lazily for the test.
- The record now hits on every edit ("cache stdx record loaded (207 modules)": a test build
  declares all of std, a build of coll 112).
- Heap after check / at SSA, max RSS: neither 164 / 196 MB, 184 MB; record only 169 / 197 MB,
  184 MB; record and lazy trees 59 / 167 MB, 202 MB (the Mac under load 115, wall not comparable).
  So check's heap falls by 105 MB, but the peak moves to SSA: a heap snapshot at the end of the
  front end holds 83 MB of parse trees, now those of the 22 from-source modules (json and every std
  module importing it, parsed whole for the SSA builder).
- Next: the from-source set limited to the modules the test program reaches (their store code is
  stale only if linked), then measure the peak again; without that, items 1 to 2 move the peak
  rather than lower it.
