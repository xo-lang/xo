# 0380: A std module's checked interface is cached, so an edit does not parse and check std again

Status: Accepted (owner, 2026-10-26 ~14:30; proposed by Xo Lang 3; built by Xo Lang 1 in two steps: (a) std modules' body check through the module check cache, (b) declaring a std module from its entry without parsing it). Follows the edit
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

## Step 1 result (2026-10-26): the bound, measured with no code change

llvm-fib's own module is nine lines, so everything the check and the parse do in an edit of it is std:
the bound needs no stub. Twenty edit rebuilds profiled at 100 Hz (XO_PROF=cpu of xo itself; the
top-30 cumulative rows summed per run; average wall 311 ms, average CPU 515 ms, two to three threads):

| where | CPU ms a run | share of CPU |
|---|---|---|
| parse of the 195 files (parse_prefetch tasks: parse_file_mode 89.5, of it lexer.lex 17.5) | 89.5 | 17% |
| the check (build_native's own thread; 57 ms of wall, 186 MB of the heap) | about 57 | 11% |
| cleanup of the decoded functions (ssa.cleaned 61.5, decode_func 43.5) | 61.5 | 12% |
| list growth and copies in all of it (obj.rl_grow* 54 to 61, rl_copy 17.5, rl_new 92 cumulative) | 55 to 90 | 11 to 17% |
| alloc.alloc (first touch of fresh pages, 28,000 page reclaims) | 113 | 22% |

So the interface entry removes the parse (89.5 ms of CPU, about 30 ms of wall across the tasks) and the
check (57 ms of wall), and with them most of the 186 MB: an upper bound of 85 to 90 ms of the 330 ms of
wall and 150 MB of the 298 MB; the entry's own load costs part of that. The next piece is
not this decision: `ssa.cleaned` decodes and cleans 1,300 functions of which 10 changed (61 ms of CPU, 17 of wall).

## Step (a) result (2026-10-26, Xo Lang 1): null

Std modules let through the module check cache (check/modcache.xo mod_cacheable without
`is_std`; the SSA builder records std modules' tasks and budgets scans as false so their
entries are written), XO_CHK_CACHE=auto, llvm-fib edit rebuilds with every cache warm:
`cache chk 25/26 modules` skipped, but `phase check` stays 55 ms and the heap after it 200 MB
(55 ms and 196 to 208 MB without). Checking std's function bodies is not the cost. A profile of six
such edits (XO_PROF=cpu of xo at 2000 Hz, CPU per edit): parsing the 195 std files about 75 ms
on the prefetch tasks (about 30 ms of wall; lexing 31 ms of it, lexer.emit's allocation the largest
part), check_program about 47 ms (Checker.run 24, resolve_sigs 9, declare 7, load_native 6). So the
gain this decision bounds is all in step (b): declaring a std module from its entry without lexing,
parsing, declaring or resolving its files, about 45 to 60 ms of an edit's wall (about 250 ms after
the day's back-end changes, f365d3eeb and the others) and most of the 200 MB. Step (a) is not
pushed (it writes entries and buys nothing alone; branch lang1-0380). Next: a spike measuring how
fast a serialized declaration table of one large std module (json or http) loads against its
parse and declare, the go or no-go for (b)'s format.

## Step (b), staged (2026-10-26, Xo Lang 1)

A spike first: 12,000 declaration records (std has about 11,500 top-level declarations: 8,018
functions and methods, 2,986 constants, 413 structs, 61 enums, 30 interfaces), as tab-separated
lines with every type interned into a map, decode in 2.2 ms against about 75 ms of CPU of parsing and
23 of declaring std an edit: go.

Step (b) lands in three stages, each on its own:

1. The parse's binary form (compiler/parser astcodec.xo): encode_parse and decode_parse, exact
   inverses over the whole tree (every declaration, statement, expression, pattern and type, every
   span, flag and byte string), the lazy parse's body ranges (decision 0351), the interface hash and
   the diagnostics; a varint and length-prefixed form whose first field is AST_CODEC_VERSION, and a
   decoder that turns any malformed input into None (a miss), never a wrong tree. Gate:
   astcodec_test.xo round-trips a sample of every construct, a file that does not parse, damaged
   bytes, and (with --real and XO_AST_CODEC_FILES) every .xo file of std, compiler/ and the LLVM
   goldens, 1,076 files, lazy and full: the decoded tree dumps (dump_result) and encodes the same.
2. Std's parses from the back-end cache (opt in, XO_IFC_CACHE=1): the parse prefetch and
   check.parse_entry read a std file's entry, keyed by the SHA-256 of the file's bytes, the parse
   mode (lazy, sig) and AST_CODEC_VERSION, and parse and store it on a miss. The checker still
   declares, resolves and checks everything, so no whole-program result can go missing (the hazard
   of gotchas/module-check-cache-skips-whole-program-passes.md does not arise). Expected: most of
   the parse's 75 ms of CPU, about 30 ms of an edit's wall, and allocation churn.
3. The default, after the identity and timing gates; then declaring std modules from entries (the
   checker's declarations serialized, types by stable names with a remap on load), the rest of the
   bound (resolve, declare, about 23 ms of CPU, and most of the 200 MB).

Stage 1 landed (7b32ec87e). Stage 2 result (2026-10-26 18:08, Xo Lang 1): small, not pushed. With
XO_IFC_CACHE=1 against the same xo with it off, alternating edits: llvm-fib edit about 305 to 295 ms
(check 58 to 51 ms), the HTTP server about 610 to 595 ms, peak RSS unchanged. The parse already ran
on prefetch tasks off the critical path, and a decoded tree allocates as much as a parsed one, so
the 75 ms of parse CPU was mostly not wall time. An opt-in flag worth about 10 ms was not landed;
the branch is kept as refs/recover/lang1-0380-stage2. Declaring std from entries (stage 3) is what
remains of the bound; the edit loop's next lever is decision 0382.


## std/testing only where a test block is (2026-10-27, Xo Lang 1)

An oracle of the edit's critical path (decision 0382's ceiling runs) found the largest share of the
check's 54 ms was not std as such but std/testing: load_prelude loaded it in every build, for the
predeclared `os`, `expect` and `testing` of test blocks, and it imports std/http, sql, log and json,
which bring their implementations and the HTTP, TLS, SQL, flate and DNS runtime modules. llvm-fib
parsed and declared 195 std files; 59 hold code its binary runs. A build or run (skip_tests,
decision 0348) now loads std/testing only when a program file has a test block (check/load.xo
load_testing_if, before load_native); `xo check`, vet, fix, the language server and `xo test` load
it as before, and a build still checks inline test blocks (cmd/xo TestBuildInlineTestBlocks).
The oracle (same xo, the load skipped, quiet, n=8): llvm-fib edit 277.9 to 241.8 ms (check 55 to
28 ms, 195 to 97 files parsed), the HTTP server 577.6 to 565.1 (it imports http and json itself;
check 56 to 52, 197 to 179 files). The change itself, quiet --ab against an xo built at its parent (n=8): llvm-fib edit 282.9 to
243.8 ms with identical output; the HTTP server 588.6 to 587.4 ms, unchanged, since its main.xo
has an inline test block and so still loads std/testing (the oracle's 6.5 ms there is the owner's
question of whether a build skips test blocks altogether). The prelude imports native/llrt/cabi_net
now: a build without std/testing lost the http import that had brought it in, and os.net's rt.*
calls lower to its exports (repotest TestRtExportsImportedByPrelude;
gotchas/implicit-std-testing-hid-prelude-import.md). Stage 3 (declaring std from entries) is
measured on top of this.

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
