# 0236: Fast native test builds: LLVM tests at -O0, the test binary cache on by default

Status: Accepted (lead, 2026-10-15, test speed work the owner approved),
implemented.

## Context

A profile of `xo test` (bench/results/xo-test-time-2026-10-15.md) on
selfhost/prune, selfhost/lexer, selfhost/check, and
examples/notes-service, cold, warm, and after an edit, on arm64, LLVM,
and Go, found the LLVM backend's link step the largest cost that per
process batching (decision 0235) does not touch:

- A test binary was built at the program default, -O2 with full LTO, so
  clang took 38.7 s of examples/notes-service's 42.8 s, 1.8 s of
  selfhost/prune's 2.9 s.
- It was paid on every run: the binary cache (XO_BIN_CACHE) was off
  unless set (decision 0174), so a warm `xo test` with nothing changed
  linked the same binary again (43.2 s warm against 42.1 s cold).

-O1 and -O2 without LTO still take 29 s and 33 s of clang on
notes-service; -O0 takes 5.4 s.

## Decision

1. `xo test --backend=llvm` builds at -O0 (so with no LTO, the single
   module rule of bench/results/llvm-single-lto-2026-10-15.md) unless
   `XO_LLVM_OPT` names a level. `xo build` and `xo run` keep -O2.
2. With `XO_BIN_CACHE` unset, `xo test` caches linked test binaries in
   `<user cache>/xo/testbin` (the directory the Go tests already use,
   internal/xotool EnableTestBinCache), keyed as before (the compiler
   and its flags, every file of the build directory, the SDK, the
   runtime pack) and pruned as before (decision 0223, 6 GiB). `off`
   turns it off; other builds keep it off unless set.

## Consequences

- examples/notes-service on LLVM: 42.8 s to 11.9 s cold, 43.2 s to
  6.4 s with nothing changed, 11.4 s after adding a test.
  selfhost/lexer 6.8 s to 4.9 s cold, selfhost/prune 3.2 s to 1.7 s.
  arm64 is unchanged (its link was already 0.05 to 0.1 s).
- An -O0 test binary runs slower: notes-service's 16 tests take about
  2.5 s more than at -O2 (6.4 s against 3.8 s, both cached).
  `XO_LLVM_OPT=O2 xo test` gives the old binary when that matters.
- Tests no longer exercise LLVM's optimizer by default. The program
  output goldens and the self build cover -O2.

## Alternatives

- -O1 or -O2 without LTO for tests: 29 s and 33 s of clang on
  notes-service, too close to the 38.7 s.
- The binary cache on for every build: not needed for the test loop,
  and a program's builds already reuse the build directory.
- A prebuilt runtime per xo version: the runtime already comes
  prebuilt (decision 0157, `cc` of arm64 0.05 to 0.1 s), so there is
  little left to gain there.
