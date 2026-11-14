# Speculative front end entries: closures replay late, copies cost, instance names collide

Status: Observed (2026-11-14, decision 0637 stage 3, Xo Lang 1; all five)

## Symptom

1. The first speculative build of xo printed, without end, "the front end cache entry ... (closure
   modules.ModCtx.git.fn264 ...: its entry missed: absent xo.debug[Location]) did not replay ...;
   building again without it", and ran for ten minutes where a cold build takes 21 s.
2. With that fixed, fe.build took 4.3 s against 3.3 s lowering each body at its turn, with GBs of
   memmove under rl_copy and large_resize in the workers.

## Cause

1. A worker lowered a body and then its closures' jobs at once. In the serial build those jobs run
   at the end of the queue, after every body queued before them, so a closure's entry recorded at
   its owner's turn read a module older than the one its replay sees ("absent" a function another
   body makes later). A body's own entry falls back to lowering when its checks miss; a closure's
   cannot (its job's data is not kept), so the build failed, and the driver's retry made the same
   entry again.
2. The queue grows as bodies are built: the first batches covered a handful of queued bodies, and
   each batch had every worker copy the builder, whose checker and module tables (c.objs, c.ctxs,
   the name and type records) copy at each worker's first write. Thousands of batches, each with
   12 copies.

## Fix

1. A body that makes closures gets no speculative entry (it is lowered at its turn), and the
   driver's retry after a failed commit turns the workers off (fe.spec_jobs = 1).
2. The first design's batches were replaced by crawling workers on one copy of the builder per
   run_queue (decision 0637 stage 3: run_queue_crawl); the workers default to 6, or 2 for a
   program with little source per table entry (spec_workers).

## Later traps (2026-11-14, the crawling design)

3. Symptom: XO_FE_SPEC_VERIFY reported a body whose parameter type was another instance's
   (result_map_err in xo). Cause: Str and Bytes lower to one SSA type, so two instances of one
   generic get the SSA names `f[Str]` and `f[Str]#2`, and a key built from the SSA names gave a
   worker's entry for one to the other. Fix: spec_inst_key also carries the type arguments' source
   names; golden spec_instance_same_name and TestFrontCacheSpecSameNameInstances cover it. Replay's
   type checks rejected the wrong entry in the small program, so only verify mode showed it.
4. Symptom: aiplatform and compute (300,000 or more checker types) were slower with more workers,
   and every program paid 0.2 to 0.35 s that no CPU profile showed. Cause: each worker copies each
   table it writes (c.tys first), and the main builder's first write to each table it shares with
   the workers' snapshot copies that table. Fix: spec_workers builds serially when the keyed source
   is under 9 bytes per table entry; a chunked c.tys would cut the copies.
5. Symptom: comparing a worker's entry with the main builder's byte for byte (fe_same_numbered)
   reported differences in bodies that build the same code. Cause: a worker's entry pins the helper
   functions it made itself (the main builder already had them) and can name an instance `#2`.
   Fix: verify replays the worker's entry over the main builder's body and compares the functions
   (spec_verify_replay, fe_func_diff), not the entries.

## A worker's entry misses as a stale snapshot, not as a cache bug (Observed 2026-11-14)

Symptom: TestFrontCacheConsts failed on host 3 (`fe miss.absent 1` in a library build) with the
default workers and passed with `XO_FE_SPEC=0`. The miss was a std body's entry (dwline's
`zero_val.native.dwline.Lines`), not the test's library.

Cause: a worker lowers a body from a snapshot older than the body's turn. Its entry makes a
function (a zero value helper) that the main builder made in between, so the replay's check
rejects it (absent), and the main builder lowers the body itself: correct output, one wasted
speculation. fe_miss counted it with the cache's own misses, which the test treats as a cache bug.

Fix: a spec replay's misses count as `spec.miss.WHY` (FeCache.spec_replay), so `miss.*` stays
the cache's own entries.
