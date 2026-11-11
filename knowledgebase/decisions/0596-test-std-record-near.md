# 0596: An `xo test --std-module` edit declares std from its record

Status: Proposed (Xo Lang 2, 2026-11-10; the perf page's "test builds use more memory" row; revised the same day, see "Root cause")

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

## Root cause and revised decision (2026-11-10, after the prototype)
The peak at SSA was not the from-source set. `--phase-times` showed `phase std_trees_rebuild` on
every test edit, first triggered by `native/prelude.str_trim_prefix`: std's test variant (decision
0436) held no facts for std's own functions. ssa build.xo build()'s test branch (b.tests) ran
build_tests and declare_exports but never declare_dump_roots, so XO_STD_ROOTS rooted nothing in
test mode: the variant's facts were 299 KB with 207 prelude and 0 json functions, against the
build variant's 2.9 MB with 5,161 and 269. Every std function a test reached was lowered from its
body, met a pending tree (contract.xo body), and the whole front end was built again with every
std tree joined.

Change (branch l2-0596):
1. build()'s test branch declares std's roots after the tests (declare_dump_roots), as the main
   path does. The test variant's facts now match the build's (2.93 MB, 269 json functions).
2. `xo test` takes std's declaration record (items 1 to 3 above, the record without the
   from-source set) and joins std's trees lazily (trees_lazy), both on by default; the module
   check cache's skipping stays off for tests (mod_skip_on), its directory is still set (the
   record's entries live there).
3. `phase std_trees_rebuild` also prints the first body that triggered it.
4. With std's test variant now holding code for every std function, a `--std-module` program
   defines functions the variant's object defines too (the tested module and its importers,
   built from source): the variant's object defines every function and C ABI export weak
   (ssa Module.weak_all, std_obj in test mode), so the program's strong copy wins the link
   (Mach-O, ELF and COFF linkers already take a strong definition over a weak one). The Mach-O
   store link drops the compact unwind entries of the store's replaced copies
   (uncoalesced_unwind, now also for store objects, matching by address: a store object's
   symbols carry renumbered section ordinals, its section relocations the original ones).
5. std_source_set (compiler/xo stdcache.xo) read only `use std/...` lines and took
   std/native/llrt as one module: a test of a runtime module (native/llrt/md5) built only that
   module from source, and its importers (crypto) came from the record without its types
   (XO0601). It now reads `use native/...` lines and takes each llrt file as its own module.
6. (2026-11-11, Xo Lang 3's report from std/textproto) std_source_set read the `use` lines of
   std/native's sources and of the merged modules' std/NAME.xo, not the declaration file
   std/NAME.xo of a module not merged: std/http.xo imports std/multipart and std/native/http.xo
   does not, so `xo test --std-module=multipart` (and any new module such a declaration file
   imports, textproto) left http out of the set. Without a record yet, http was declared and its
   types resolved in the record's block before multipart (in the set) was declared past it:
   XO0601 "module `multipart` has no type `Form`". It now reads every std/NAME.xo and names both
   "native/NAME" and "std/NAME" for such a module (multipart's set: 10 to 17 modules), but not the
   prelude's (`use std/task`: with it in the set every runtime module's test lost the prelude).
   All std modules with tests, Mac: 62 of 63 pass (main: 60; mime, multipart, url failed with
   XO0601); mime now compiles and fails one test, detect_content_type, only when separately
   compiled (XO_SEPCOMP=0 and LLVM pass): the program's constant numbers shared std's (decision 0388
   "Constant numbers", fixed the same day); with that fix all 63 pass.
   TestSepcompStdModuleEdits runs `--std-module=multipart` after building the variants.
Every std module with tests (62) passes `xo test --std-module` with the change on the Mac.
Step 3 of the prototype (the from-source set limited to reached importers) is not needed: with
the facts present no from-source importer's body is lowered unless the test reaches it.

Mac (darwin/arm64, under other agents' load), `xo test --std-module=native/json` after an edit of
json.xo, three edits, 10 of 10 tests pass each time:

| | wall | max RSS | heap after check | SSA phase |
|---|---|---|---|---|
| main | | 199 to 214 MB (prototype runs above) | 150 to 164 MB | std_trees_rebuild every edit |
| roots fix only | 0.19 to 0.24 s | 194 to 202 MB | same | 53 to 58 ms, no rebuild |
| roots fix, record, lazy trees | 0.15 s | 108 to 109 MB | 67 to 68 MB | 38 to 39 ms |

The first edit after the variant is built is 0.21 s and 197 MB (the record is written then).
go test's json edit on linux/arm64 is 158 MB (perf-gaps-2026-11-10.md). Cost: `xo dev stdcache
--test` now builds std whole in test mode, 8.0 s and 1.53 GB peak (conc=0) and 9.6 s and 1.85 GB
(conc=1) on the Mac, against 2.1 s and 1.45 GB before (it compiled little); it runs in the
background once per std and xo.

### Measured on linux/arm64 (host 1, 2026-11-10)
`xo bench --ab` of `xo test --std-module=native/json` after an edit of std/native/json.xo (each run
appends one function to its side's json.xo, then runs the test; each side its own xo, std copy,
HOME and caches, std's test variant built first), base fd75d03e5 (A) against eb5ada8f2 (B), 9
rounds, 2 warmups, host load 9.7 to 9.8 on 12 CPUs throughout (no round marked loaded;
ab0596-job.sh in the agent's notes):

| metric | A median | B median | B/A | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 187.2 | 142.7 | 0.762 | 0.743 to 0.772 | better |
| cpu_ms | 251.9 | 178.8 | 0.710 | 0.677 to 0.732 | better |
| max_rss_kb | 230,484 | 121,384 | 0.527 | 0.522 to 0.531 | better |

go test's json edit on the same host: 158 MB (perf-gaps-2026-11-10.md section 3).

## Measure (before Accepted)
- Peak RSS and the heap after check of a test edit of json, base64 and a leaf module, 3 edits each,
  against main, on linux/arm64 host 1 (`xo bench --ab` with release-vs-go.sh's edit guard). Target:
  json's edit at or under go test's 158 MB.
- Identical test output and binaries against main (scripts/identity on the test binary).

## Consequences

- With its roots declared, std's test variant lowers and inlines all of std/testing, which
  exposed an inliner case: in `--conc=0`, `testing.fake_db` inlined `sql.memory` and its
  `new_db`, a conc_off stub that faults holding the moved tables map, so the inlined module did
  not verify and the whole test variant fell back to no inlining (strict mode failed it:
  TestSepcompStdModuleEdits). conc_off stubs are never inlined now (ssa.CONC_OFF_FAULT,
  gotchas/conc-off-stub-inlined-holds-moved-args.md); all four std objects (build and test,
  conc 0 and 1) verify under XO_INLINE_STRICT=1.
- std's test variant is now std built whole in test mode: `xo dev stdcache --test` takes 8.0 to
  10.1 s and 1.53 to 1.85 GB peak per concurrency setting on the Mac (was about 2.1 s and 1.45 GB,
  when it compiled little), once per std and xo, in the background on a first `xo test`; its facts
  grow from 299 KB to 2.9 MB.
- Gate (host 3, eb5ada8f2): internal/arm64 passes; cmd/xo fails TestProtoGen and
  TestXoTestStdModuleNames, which fail the same way at the base fd75d03e5 (on the Mac too), and
  TestDebugDAPAttach, which passes on the Mac at both revisions.
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
- Which of the set the test links (Mac, the json test binary after an edit, `nm`, function symbols
  `_xo_f_<file>_2e<name>` grouped by prefix): of json's 21 std importers only trace (4 functions)
  and the runtime (std/native/llrt, which imports json) appear; amqp, cbor, http, metrics, otlp,
  protojson, sql, sqlite, template, tls, toml, websocket, x509, yaml and the others link nothing,
  yet each is parsed whole and built from source on every json edit. So step 3 of the decision:
  the from-source set is the importers the test program reaches, from std's facts (the stdf call
  graph) rather than the `use` lines alone; an importer it does not reach keeps its store code,
  which the link drops (decision 0388's dead stripping reads only the live atoms' relocations).
