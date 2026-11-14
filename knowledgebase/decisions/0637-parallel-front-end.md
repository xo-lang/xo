# 0637: The front end builds function bodies on the task pool

Status: Accepted (owner, 2026-11-14, through the lead: stages 1 and 2 under the build-time plan; stage 3
approved after them, its first step measuring the replay hit share with XO_FE_STATS=1 on aiplatform/apiv1
and the self build; gate: decision 0636's determinism runs (XO_EMIT_JOBS=1, 2, 7, default,
XO_THREADS=3, XO_MUTEX_YIELD=1, 10 cold runs each), the fixed point, and the self build under
XO_MAX_MEM with the checker copies' memory reported). The second of the re-profile's three time cuts
(~/xo-lead/notes/lang1/retime-rank-2026-11-14.md). Compiler internal: no language or output change.

## Context

ssa.Builder.run_queue (compiler/ssa/build.xo) builds every function body of the program on the main
fiber: a cold build of gcp's aiplatform/apiv1 tests spends 2.9 to 3.1 s in fe.build on host 1, xo's
own build 4.5 s (6.4 s on host 2), with 11 of 12 CPUs idle. The queue grows while it runs (a body's
calls declare the functions they reach, declare0 gives each its index in m.funcs in discovery
order), and a body's lowering writes order-dependent state shared by every body: the module's type
and function tables (m.types, m.funcs, m.order), the closure counter (`.fnN` names), constant,
spawn site and interface type ids, thunks, the checks list, diagnostics, and the checker (lazy
parse, lazy check, c.tys). A plain parallel loop would renumber all of it.

Where run_queue's time goes (host 2, cold, CPU profile at 1 kHz, ~/xo-lead/notes/lang1/pfe-h2.txt,
main 114f8dce2):

| part of run_queue | aiplatform/apiv1 | self build |
|---|---|---|
| all | 3.36 s | 8.90 s |
| lowering (body_of, test_body) | ~2.1 s | 2.93 s |
| lazy check (check_fn_decl) | 0.44 s | 2.22 s |
| lazy parse (force_body's parser.parse_body) | 0.41 s | 0.92 s |
| front end cache entry (fe_entry, fe_seal) | 0.27 s | 1.70 s |

## Decision (in stages)

1. Parse ahead. parser.parse_body(path, src, span) reads only the file's text. When a Body job is
   queued for a function whose body is still lazy, a task of the pool parses it; force_body takes
   the finished tree (or parses itself when the task has not finished), so every tree and every
   diagnostic is the one the serial build makes. Expected: 0.4 s (aiplatform) and 0.9 s (self) off
   the main fiber.
2. Encode entries off the main fiber. fe_entry records the body's reads and events from m.fe_log at
   once (they describe the module at that point), and encodes the body (encode_func) and seals the
   closures' entries on a task; the build joins the tasks before the stores and before body_ids
   reads body_entries. XO_BACKEND_CACHE=verify keeps the inline encode (it compares at once).
   Expected: 0.3 s and 1.5 s.
3. Speculative lowering, committed in queue order. Workers lower the next queued bodies (with their
   closures) against a snapshot of the builder and their own copy of the checker, producing front
   end cache entries exactly as a miss records one; the main fiber commits them in queue order with
   fe_replay, which checks every read before it changes anything and replays the body's events so
   that types, functions, closures, constants and sites get the numbers the serial build gives them.
   A body without an entry (fe_fixed: a new generic instance, interface ids, thunks, diagnostics,
   Inner/Test/Wrap jobs), or whose replay misses because an earlier body of the same batch made
   what it read, is lowered on the main fiber at its turn, as today. The equivalence check exists:
   fe_same_numbered (verify mode). Its gain is bounded by the share of bodies that replay; the
   first step of stage 3 measures it (XO_FE_STATS=1 on both programs).

Stages 1 and 2 give byte-identical output by construction (the same trees, the same entries). Stage
3 needs the determinism runs of 0636 (XO_EMIT_JOBS=1, 2, 7, default, XO_THREADS=3, XO_MUTEX_YIELD=1,
10 runs each, cold) and the fixed point.

## Results (2026-11-14)

- Stage 1 (parse ahead; branch lang1-pfe, 7036cfe70, XO_PARSE_AHEAD=0 to compare): output
  byte-identical in all runs (host 2, cold: the self build 4b365f3b... in 10 runs, aiplatform/apiv1
  cc7376ad... in 9, across XO_EMIT_JOBS=1, 2, 7, default, XO_THREADS=3, XO_MUTEX_YIELD=1; host 3
  the same). It parses 13,558 of the self build's 13,766 lazy bodies ahead, but fe.build (Mac, cold,
  three alternations) is 3.7 to 4.4 s without it and 3.6 to 4.1 s with it: within noise, for about
  100 MB more heap (the tasks' own heap chunks, decision 0448; a batch of 64 bodies instead of 256
  changes neither). parse_body is about 0.4 s of the Mac's 3.8 s. Not pushed alone: stage 3's
  workers parse their bodies anyway.
- Stage 2 (entry encoding off the main fiber): a second profile (host 3, main at 5e8afa8f5, the self
  build cold, ~/xo-lead/notes/lang1/pfe3-h3.txt) puts fe_entry at 0.95 s of CPU, encode_func 0.32 s
  of it; fe_seal is 6 ms and copy_words 71 ms, where the first profile showed 0.97 s and 0.9 s. The
  part that can move (the body's encoding) is about 0.3 s of a 6 s fe.build: also below what the
  stage is worth alone. Folded into stage 3 likewise (its workers encode their own entries).
- So the first two stages do not reach the ranking's estimate; the time is in lowering (2.9 s) and
  the lazy check (2.2 s), which only stage 3 runs in parallel. Its estimate starts from those two.
- Stage 3's coverage, first step (XO_FE_STATS=1, cold): the self build keys 13,711 of 13,997 bodies
  and skips 258 at recording (gen 238), so about 96% could replay; aiplatform/apiv1 keys 6,499 of
  11,897 (the rest are generic instances, which have no key, fecache.xo) and skips 999 (gen 993), so
  about 46%. Stage 3 needs generic instances keyed (the generic body's key and the type arguments
  by name) to cover aiplatform.
- Stage 3, a first design (branch lang1-fespec, not pushed): batches of queued bodies, each worker
  copying the builder per batch and lowering every w-th body. Two traps (gotcha
  fe-spec-copies-and-closures.md): closure-making bodies' entries did not replay and the driver's
  retry made them again without end, and batches of a few bodies copied the builder thousands of
  times (fe.build 4.3 s against 3.3 s serial). It covered no generic instance, so aiplatform could
  not gain.
- Stage 3 as built (branch lang1-fespec3; ssa/build.xo run_queue_crawl, spec_crawl, spec_workers,
  spec_claim_offer, spec_one, spec_inst_key, spec_verify_replay): crawling workers. The builder is
  copied once per run_queue (`let snap = b`), and each worker takes function objects from a shared
  work channel, lowers the body on its copy (check_lazy, run_job0, fe_entry, fe_seal), sends the
  entry on a results channel, then lowers right away the generic instances that body declared
  (keyed for this build only by spec_inst_key: the generic's object and its type arguments' names,
  plus the source names, since Str and Bytes lower to one SSA type and `f[Str]`, `f[Str]#2` both
  arise) and offers the plain callees it declared to the work channel. So the workers run ahead of
  the queue by following the call graph, instead of waiting for the main builder to declare. The
  main builder claims its next 32 queued bodies under one lock (SPEC_CLAIM; a body a worker has not
  started is the main builder's) and offers the rest; at each Body job it drains the results without
  a lock and replays the entry as a hit (a miss, or no entry, lowers it as before). fe_replay gives
  every type, function, closure and site the serial build's number whatever the worker held, so the
  output is the serial build's. Excluded and lowered at their turn: a body that makes closures or
  reports anything (spec_one, spec_reports), a skipped module's or std extern's body, one with a
  cache entry already.
- The snapshot's fixed cost: the main builder's first write to each table it shares with the
  workers' copy copies that table. About 0.24 s on the Mac and 0.35 s on host 1 for aiplatform
  (bisected: the checker 90 ms, the module 23 ms, the rest over the builder's maps); it shows in
  neither CPU profiles nor sys time. Each worker likewise copies the tables it writes (c.tys first):
  for a program whose checker holds 300,000 or more types, more workers cost more than they save.
- The worker count (spec_workers, the default when XO_FE_SPEC is unset): min(emit jobs, 6), but
  none (the serial loop, no snapshot) when the keyed bodies' source is under 9 bytes per entry of
  the tables a worker copies
  (spec_tables: the checker's objects, types, records and names, the module's functions and types;
  both are known when run_queue starts, a sum over fe.keys). 8 programs on the Mac (cold, fe.build
  ms, XO_FE_SPEC=0, 2, 6; ratio = keyed source bytes per table entry):

  | program | checker types | ratio | off | 2 | 6 |
  |---|---|---|---|---|---|
  | gcp aiplatform/apiv1 tests | 305k | 6.4 | 2299 | 2336 | 2538 |
  | gcp compute/apiv1 tests | 484k | 5.2 | 2202 | 2200 | 2723 |
  | xo (compiler/xo) | 78k | 12.1 | 3324 | 2130 | 1491 |
  | gcp bigquery tests | 39k | 13.7 | 911 | 782 | 653 |
  | gcp bigquery/apiv2 tests | 33k | 15.2 | 867 | 820 | 702 |
  | gcp storage tests | 32k | 13.8 | 924 | 864 | 684 |
  | bench http server | 7k | 24.9 | 812 | 634 | 493 |
  | std json tests | 3k | 49 | 155 | 119 | 93 |

  The two programs below 9 lose at 6 workers and are about neutral at 2 on the Mac; every program
  at 12 or more gains at 6. On host 1, 2 workers lost on both (below), so they build serially. An
  XO_FE_SPEC the user sets is taken as it is.
- Equivalence: XO_FE_SPEC_VERIFY=1 lowers every body on the main builder, replays the worker's
  entry over it and compares the two functions (fe_func_diff: values, types, aux, args, positions,
  debug records; the names table aside), then restores what the replay changed. A byte comparison
  of the entries (fe_same_numbered) reports benign differences: a worker's entry pins the helper
  lines it made that the main builder already had, and an instance named `#2` on one side. The
  replay comparison passes on aiplatform at 6 workers (10,611 same, 1,814 entries miss and are
  lowered) and on xo at 2 (13,411 same, 357 miss) and 6 (13,396, 548); with the source names left
  out of spec_inst_key it reports the wrong instance replayed (result_map_err's parameter type, xo,
  XO1510 before the fix). XO_FE_SPEC_WAIT=1 makes the speculation deterministic for tests (every
  keyed body goes to the workers and its turn waits for them). The golden spec_instance_same_name
  (internal/llvm/testdata/golden, env XO_FE_SPEC=4 WAIT VERIFY) and
  TestFrontCacheSpecSameNameInstances (internal/arm64) build two instances whose SSA names collide
  (a generic over Str and Bytes): same output, verify passes, instance hits, and the binary equals
  XO_FE_SPEC=0's.
- Host 1 (linux/arm64, cold, default caches each fresh, ~/xo-lead/notes/lang1/spec-h1c.txt, branch
  at 04c22ee54 where a program under the bound took 2 workers): fe.build ms, runs and median, and
  max RSS:

  | program | XO_FE_SPEC | fe.build (median) | max RSS |
  |---|---|---|---|
  | xo (compiler/xo) | 1 | 4392, 5399, 4966 (4966) | 3.68 to 3.81 GB |
  | | 2 | 3259, 3419, 3910 (3419) | 3.77 to 3.88 GB |
  | | 6 | 2538, 2360, 2102 (2360) | 3.80 to 3.96 GB |
  | | 12 | 3110, 2900, 2765 (2900) | 3.88 to 3.91 GB |
  | | default (6) | 2497, 2312, 2202 (2312) | 3.81 to 3.86 GB |
  | aiplatform/apiv1 tests | 1 | 3172, 3110, 3847 (3172) | 2.16 to 2.24 GB |
  | | 2 | 3393, 3323, 3146 (3323) | 2.18 to 2.22 GB |
  | | 6 | 4184, 4186, 3458 (4184) | 2.79 to 2.83 GB |
  | | 12 | 4461, 4415, 4467 (4461) | 3.95 to 4.00 GB |
  | | default (2 then) | 4107, 3346, 3356 (3356) | 2.17 to 2.22 GB |

  The other programs (serial, then the default; two runs each): http 1207, 1607 to 1217, 907; json
  243, 268 to 200, 209; bigquery 1638, 2396 to 1216, 1227; bigquery/apiv2 1757, 1719 to 1219, 1838;
  storage 1874, 3323 to 1077, 1171; compute 4862, 4255 to 5073, 7322 (2 workers then). The host's
  noise is wide (xo serial 4.4 to 5.4 s), but aiplatform and compute at 2 workers are slower in
  every pairing, so a program under the bound now builds serially (c9d14eaeb) and is the serial
  build by construction. Every program's output is byte-identical in every run (one SHA-256 per
  program over 81 builds): the worker counts, the determinism set of 0636 (5 cold runs, XO_EMIT_JOBS=1,
  2, 7, XO_THREADS=3, XO_MUTEX_YIELD=1), XO_FE_SPEC_WAIT, XO_FE_SPEC_VERIFY at the default and at 6
  workers (30.8 to 40.8 s, all same), and the self build under XO_MAX_MEM=8G (fe.build 2032 ms,
  max RSS 3.85 GB).
- Not in this stage: generic instances' entries are kept for this build only (the cache keys no
  instance; effect-only generics, which the cache could key, are a separate
  commit with its own measurement). A worker's entries pin the helper lines it made, so a warm build after an edit can miss
  more than a serial cold build's entries would; a chunked c.tys would cut the per-worker copies
  that keep the generated clients serial; verify mode takes 31 to 41 s on linux/arm64.

## Consequences

- Stages 1 and 2 together are expected to take about 20% of run_queue for aiplatform/apiv1 and 30%
  for the self build: less than the ranking's estimate (2.1 and 3.3 s), which needs stage 3's lowering
  and checking in parallel.
- Stage 3 makes a cold front end do the work of a warm one's replay on top of the speculative
  lowering: its gain is (lowering time minus replay time) times the workers, less the misses.
- The checker copy per worker (c.objs, about 4.3 MB for xo, copied on write) and the c.tys growth
  are stage 3's memory cost; it is measured against XO_MAX_MEM in the gate's self build.

## Alternatives

- Lower bodies in parallel with local tables and a merge that renumbers (a second numbering pass
  over every value's type id, function name and counter): the fe cache's replay already is that
  merge, with its checks, so stage 3 reuses it instead of writing a second one.
- Parse every lazy body of the program ahead (not only queued ones): simpler, but parses bodies the
  build never reaches (aiplatform's SDK has many) and holds their trees (decision 0351 dropped
  them for memory).
- Keep fe.build serial and only make it cheaper: fe_seal's copies (copy_words 0.9 s of the self
  build) are worth a separate look either way.
