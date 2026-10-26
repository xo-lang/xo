# 0382: The passes between the caches become per function and per part in an edit rebuild

Status: Proposed (Xo Lang 3, 2026-10-26, for the owner; nothing built). The second half of the
edit rebuild work (decision 0380 is the front end half): after the front end, every cache hits
and an edit of one function still costs the rest of the 330 ms because each phase walks all
1,300 functions and all the emitted text.

## Context

llvm-fib, one added function, every cache warm (see decision 0380 for the method): the phases after
the front end cost, in wall ms, prune_unwind 8, add_landings 6, cleanup 17, specialize_args 4,
insert_rc 28, assume 7, inline 24, dce 3, inline.verify 18, emit 4, hollow 30, undef 25, assemble 10
(4 of 4 objects reused), link 14. Twenty profiles (100 Hz, CPU ms a run, tasks included): the
cleanup of the decoded functions 61.5 (decode_func 43.5, renumber about 40, cleanup_func 30),
force_lazy's decoding 50, the key texts of the inline and function keys 30, ssa.key_func 20, may_fault
10; hollow and undef have no hot line (starts_at 6.5, undefined_xo 3.5, memmove 17 and alloc 8.5 in
the whole run): they scan every line of all the parts (MBs of text), 28 ms and 21 ms.

What an edit changes is one function (10 of 1,289 bodies miss the front end cache). What the
passes do is decode the 1,289 other bodies from their front end entries, clean each, then ask the
reference counting, inline and function caches whether their entries hit (they all do, and the
functions stay lazily undecoded from there on, decision 0283 stage B): the decode and the clean run
only to compute the key those caches look up. Hollow and undef then read the text of every part,
though 3 of 3 parts' keys say nothing but main's changed.

## Decision (proposed)

1. Chain the keys from the front end entry. A body's front end key (content, interface and, for a
   replay, its recorded state) already says what the body is before cleanup. The cleanup,
   reference counting, inline and function keys are derived from it by hashing it with what each
   stage also reads (the callees' signatures and the interprocedural facts prune_unwind,
   fold_known_calls and add_landings use: may fault, never returns, immortal results; the same facts
   rc_key and fn_key hash today), without decoding the body. A function whose chained keys all hit
   is never decoded: its post-cleanup form is a cache entry ("cln", read lazily like rc's), and the
   decode and clean run only for the functions that miss (the edited ones and what depends on them).
2. Per-part summaries for hollow and undef. Beside a part's cached text (parts 3 of 3 reused) the
   cache keeps its summary: the part's functions with their labels, the symbols each references,
   the global labels it defines, the weak declarations, and the roots it contributes. Reachability
   and the undefined symbol check run over the summaries (small), and only a part with an unreached
   function is rewritten; the summary of an edited part is recomputed from its text, as today.
3. Soundness: each stage's chained key must cover every input that stage reads; the proof is
   XO_BACKEND_CACHE=verify (decode and clean everything and compare with the chained results),
   output identity (scripts/identity) on 400 programs, the editing tests (a callee's effect, a struct
   field, a constant; TestBackendCacheEditIdentity and TestFrontCacheInterfaceEdits shapes), the
   fixed point. Opt in (XO_CHAIN_KEYS=1) until verify is clean over the golden corpus.

## Correction after reading the code (2026-10-26, before any build)

Step 1 above cannot chain the keys "without decoding" the replayed bodies. fe_replay (ssa
fecache.xo) is not a decode of an independent function: it commits the entry's events in order into
the builder (types made, functions made whole, generic instances, constants, site ids, renames:
"so the module grows as a serial build at this point grows it", decision 0271), so a body's
replay must run, in order, for the module to be what the later passes read; the 43 ms of the `ssa`
phase is that floor. And the passes before the reference counting are interprocedural: prune_unwind
drops or keeps each check by `may_fault` of its callee (a closure over the whole call graph) and
fold_known_calls folds the calls of constant functions of every module, so a function's form after
them depends on facts of its callees, not on its own entry alone; specialize_args (decision 0368)
clones callees per constant argument of their callers.

What a chained key can do, after the replay: key the replayed function once (key_func, about 20 ms
of CPU for all 1,300, a cost the rc stage already pays after cleanup) together with the callee facts
those passes read, and keep the post-pass form as a cache entry, read lazily like rc's, so
prune_unwind's rewrite, add_landings, cleanup and renumber (cleanup 17 ms of wall, 61 ms of CPU)
run for the changed functions only. That is about 30 to 40 ms of the 330, not the 120 ms estimated
above; the replay (43 ms) and the key hashing stay. The hollow and undef summaries (step 2) are
unaffected: about 30 to 40 ms of the 55. With decision 0380 (85 to 90 ms) the edit would be near
200 ms, not Go's 100 ms. The remaining floor is the replay and the whole-program walks; the way
under it is the compile server (decision 0283) or a replay that does not rebuild the whole module.

## Expected effect (to be measured)

The edit's middle phases (108 ms) fall toward the cost of the 10 changed functions and the key
hashing (a guess: 30 ms) and hollow plus undef (55 ms) toward the edited part's share (a guess:
15 ms): 120 ms of the 330 ms, on top of the front end's 85 to 90 ms (decision 0380): together the
edit would be near Go's 100 ms. Both are guesses to be checked by a prototype of one stage first.

## Measurement plan

1. A prototype of step 1 for the cleanup stage alone (the "cln" entry and the derived key), behind the
   flag: phase rows (cleanup, force_lazy), CPU profile counts of decode_func, on the llvm-fib,
   CRUD and xo edit cases; identity on a few programs; verify clean.
2. Then the reference counting and inline stages; then the function keys and verify keys.
3. Step 2 separately, as its own commit: hollow and undef phase rows and the assembly identity.
4. Quiet machine, interleaved with main, 11 reps: wall, peak RSS, phase table. A cold build and a
   warm build must not regress (the "cln" entries are written by the first build).

## Alternatives

- The compile server (0283): resident state instead of better keys; complementary.
- Parallelize the scans (hollow per part, undef per part): 3 to 4 parts, uneven, and the memmove
  and map work stays; it caps at about 2x of 55 ms.
- Skip hollow and undef for an edit that adds no function reachable from outside: unsound for the
  next edit that does.
