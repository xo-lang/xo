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
