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
3. Addendum (lead, 2026-10-15): the run cache (selfhost/xo
   testcache.xo). A native `xo test` without `-o` first computes a key
   it knows before the check: xo's identity (self_id), the backend,
   the working directory, the std directory, the command line without
   the flags that only change the run (`-run`, `-seed`, `-cases`,
   `-v`, `--json`, `--phase-times`), every `XO_` environment variable
   and PATH, HOME, XDG_CACHE_HOME, CC, SDKROOT, DEVELOPER_DIR, the host,
   the C toolchain's (and for LLVM clang's) identity, the SDK the last
   link used, and the runtime pack. Its entry in the bin cache
   (`run/<key>.m`, `run/<key>.bin`) lists every input with the SHA-256
   of its contents: every file and directory listing of the std tree,
   every file and the listing of each non-std module directory the
   program uses, and xo.mod, xo.lock, xo.work, xo.work.lock (or their
   absence) from the working directory up. When all still match, the
   stored binary runs with the stored test list, and the check, SSA,
   passes, emission, and link are skipped. A build that printed
   diagnostics is not stored (a hit never hides a warning), nor one
   whose inputs changed while it ran. One entry per key: going back to
   earlier contents builds again.

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

- With the run cache, a run with nothing changed costs the key and the
  input hashes (25 to 70 ms) plus running the tests.
- Known limit: an input the build reads outside those (a `use c` file
  outside the module directory through XO_CBIND_DIR's contents, a
  module cache directory edited in place) is not hashed; XO_BIN_CACHE=off
  avoids the cache.

- Addendum (lead, 2026-10-15): the bin cache grew to 12 GB in a few
  hours of parallel sessions, all of it younger than the prune's
  two hour keep (decision 0223), so the 6 GiB cap removed nothing. Its
  policy is now its own (selfhost/xo cache.xo prune_bins): at most every
  2 minutes after a store of a binary or a run entry, a 4 GiB cap
  (XO_BIN_CACHE_MAX bytes, else XO_CACHE_MAX_GB), least recently used
  first, entries used within 10 minutes kept (prune.Policy.keep), and
  marked used at most once a minute (prune.mark_used_within). A run
  cache hit runs a copy of the binary in $TMPDIR, removed after the run,
  so a prune never removes a binary while its tests run.

## Alternatives

- -O1 or -O2 without LTO for tests: 29 s and 33 s of clang on
  notes-service, too close to the 38.7 s.
- The binary cache on for every build: not needed for the test loop,
  and a program's builds already reuse the build directory.
- A prebuilt runtime per xo version: the runtime already comes
  prebuilt (decision 0157, `cc` of arm64 0.05 to 0.1 s), so there is
  little left to gain there.

Addendum (2026-10-21, xo-fa): `xo test --std-module=M` uses the run
cache too. It was left out when the flag came (decision 0262 part 2),
with no reason recorded; the key has the command line (the flag with
it) and the entry hashes every file of the std tree (M's _test.xo with
it), so an edit to M or its tests misses. A warm std module test had
checked and built again, 0.55 to 0.83 s (std path, url, mime). Test:
internal/arm64 TestStdModuleRunCache.
