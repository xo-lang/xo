# 0380: A std module's checked interface is cached, so an edit does not parse and check std again

Status: Proposed (Xo Lang 3, 2026-10-26, for the owner; nothing built). Follows the edit
rebuild breakdown the lead asked for after the v0.32.0 comparison with Go
(bench/results/release-vs-go-v0.32.0.md: an edit rebuild of llvm-fib takes 330 to 360 ms and peaks
at about 300 MB, Go's 100 ms and 67 MB; cold and warm builds beat Go).

## Context

Measured 2026-10-26 on the current main xo, llvm-fib after adding one function, every cache
warm (`--phase-times`, XO_CACHE_STATS=1, a 100 Hz XO_PROF=cpu and XO_PROF=heap of xo itself):

- Every cache hits: parse 195 of 195, front end bodies 1,279 of 1,289 (10 rebuilt), borrows,
  reference counting and inline entries 1,300 of 1,300, functions 520 of 520, parts 3 of 3,
  objects 4 of 4. The edit still costs 330 ms because the phases between the caches walk the whole
  program: the front end 137 ms (check 57, fe.keys 9, fe.read 3, fe.build 25, ssa 43), the middle
  108 ms (prune_unwind 8, add_landings 6, cleanup 17, specialize_args 4, insert_rc 28, assume 7,
  inline 24, dce 3, inline.verify 18), emit 4, the back 80 ms (hollow 30, undef 25, assemble 10,
  link 14) and 13 ms to find the whole-build cache missing.
- The heap goes from 14 MB to 200 MB in the check (195 files parsed and checked), to 272 MB after the
  front end's SSA, to a 292 MB peak at cleanup; peak RSS 298 MB. At exit 219 MB sit on the free
  lists (119 MB in 1 to 8 KiB blocks, 61 MB in blocks of 16 KiB and over that no other size class
  reuses): the peak is the allocator's size classes under churn, not the lifetime of the trees
  (decision 0352 frees the checker; XO_KEEP_FRONT=1 peaks at 358 MB). The process allocates 928 MB
  in 3.5 million objects for a 330 ms build; lexing and parsing are about 25% of it (the lexer's
  tokens 97 MB, lex_habits' copy of them 105 MB until 2026-10-26, number literals' builders 37 MB).
- The parse cache is not a cache of parses: parse_prefetch (bcache.xo) reads the files the
  previous check parsed and parses them on tasks, so all 195 files are lexed and parsed again by
  every build ("cache parse 195/195 reused" counts the prefetch hits). The module check cache
  (decision 0220, XO_CHK_CACHE) covers only program modules ("cache chk 0/1 modules"; std, native
  and the prelude are always checked), and `auto`, its default for one day, is opt in again
  (0220's 2026-10-22 addendum). Decision 0283 (a resident compile server) would hold the checked
  modules in memory but is not started, and costs memory the shared Mac does not have.

Go avoids this by keeping each package's export data: an edit of main compiles main and links.

## Decision (proposed)

1. A per-module interface entry for every std module (cache kind "ifc", beside "chk"): the module's
   exported declarations as the checker produces them (names, types and their layouts, function
   and method signatures with their inferred effects, constants' values, derive and enum variants,
   the generic definitions that instantiations read), keyed by xo's identity and flags, the
   module's path, the full source of its files and the keys of the interfaces it imports (as the
   `chk` key already does for program modules).
2. A build that finds every key for a module does not lex, parse or check its files: the module is
   declared from its entry. Its function bodies are checked, and its files parsed, only when the SSA
   builder asks for a body the front end cache cannot replay (the 10 of 1,289 here are all in the
   edited module; a std edit misses its own module and the modules that import it, as 0220's
   step 1 states).
3. The whole-program results a skipped module contributes are carried in its entry and replayed
   (inferred effects, never results, flow facts, uses of tasks and budgets, the zone calls):
   gotchas/module-check-cache-skips-whole-program-passes.md is the reason `auto` was reverted, and
   the entry for std must not repeat it; TestModuleCheckAuto* and a build-twice comparison of every
   golden are the gate.
4. Opt in (XO_IFC_CACHE=1) until the gates pass and a quiet window shows the numbers below; XO_BACKEND_CACHE=verify
   checks every entry against a full check.

## Expected effect (to be measured, not promised)

The check (57 ms, 186 MB) and the parse (90 ms CPU) of the 194 std modules go to a load of entries;
the heap before the SSA falls from 200 MB to the program's own modules plus the declarations; the peak
falls with the allocation (the 928 MB). A guess of 100 ms and 100 to 150 MB of the 330 ms and 298 MB;
the middle and back phases (hollow, undef, the key and verify passes: separate items) are the rest.

## Measurement plan

1. First, an instrument, no entry: how much of the check's 57 ms and 186 MB is std (a run with only
   main's module, the rest declared from a hand-made stub), to bound the gain before building.
2. The edit rebuild of llvm-fib, the CRUD server and xo itself, interleaved with main, quiet
   machine: wall, peak RSS, the phase table, allocation by XO_PROF=heap.
3. Output identity (scripts/identity) on 400 programs on arm64 and amd64; the fixed point; the
   build-twice goldens; XO_BACKEND_CACHE=verify clean; the editing tests (TestFrontCacheInterfaceEdits
   style: a std struct field, a signature, a constant, an effect).
4. A cold build and a warm build do not regress (entries are written in the first build: the cold
   cost is measured).

## Alternatives

- The compile server (0283): keeps state in memory across builds, 1 to 2 s targets for xo's own
  edit; it is complementary (the server could hold the same entries decoded) and costs resident
  memory; this decision gives the one-shot `xo build` the same win for std first.
- Cheaper lexing and parsing (smaller Tokens, text as offsets, tokens on demand): measured pieces of
  it are small (lex_habits 3% of an edit, the capacity of the token list nothing); the 88 byte Token
  and a Bytes copy per token are the structure.
- Caching the check of std as a whole (one entry for all std): invalidated by any std edit, and std
  is edited often by the agents; per module follows 0220's key rule.
