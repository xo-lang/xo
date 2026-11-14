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
