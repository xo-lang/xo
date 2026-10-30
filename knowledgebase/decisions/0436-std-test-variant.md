# 0436: std's separately compiled variant for test builds

Status: Proposed (xo-lang-4, 2026-10-30; the lead approved building it, the owner accepts with the measurements below)

Amends [0388](0388-separate-std-compilation.md): std's variants gain a test dimension.

## Context

`xo test` was left out of separate compilation: sepenv.xo's
sepcomp_default admitted only `xo build|run`. A test binary's std is not
the build's std, so the build's store could not serve it:

1. Test mode (ssa Builder.tests) rewrites every function's calls of the
   prelude's Fs, File and FileLock methods to std/native/testing's
   memfs_, memfile_ and memlock_ functions (testmode.xo mem_fs_call), and
   routes Os and Proc capability calls to the test world (TestOs,
   FakeProc). std's own bodies that use Fs compile differently.
2. The test world's roots (std/testing, native/testing, native/rttest,
   llrt_tw_*) exist only in a test build (build.xo).
3. Not a factor: det. c.det is set only for `xo build|run --seed`; a test
   binary's deterministic scheduler is the runtime's.

After 0351's lazy parse reached `xo test` (635488fae), a one-test
program's edit still compiled std whole: on darwin/arm64 its ssa phase took 187 ms of a 0.99 s edit, at 971 MB peak RSS (bench/results/test-vs-go-2026-10-30.md has the cross-language rows).

## Decision

std's variants are keyed (target, conc, test). The test variant is std
built in test mode with the test world's roots:
`xo dev stdcache --test [--conc=0|1]`, which runs `xo dev stdobj --test`
(an `xo test` of a one-test program with XO_STD_OBJ, then with
XO_STD_ROOTS for the facts; XO_BIN_CACHE=off so a run-cache hit cannot
skip the build, and the test binary is not run). std_cache_key adds
`test=1`, and the released name gets a `-test` suffix.

sepcomp_default admits `xo test` under the same conditions as build
(native backend for the host, no --release, --race, cc linker or
assembler, no changed pass or std code), and picks the pair for the
program's concurrency, as build does. Not `--std-module`: the module
under test is compiled with its tests' private access, so it is not
taken from the store (item 2 of the test speed queue builds only that
module from source).

A miss behaves exactly as build's: the whole program is built and the
variant's background build starts.

## Consequences

- Cost: one more store per concurrency setting and target (darwin/arm64:
  a 6.25 MB store and 275 KB of facts each, two per target), and one more
  background build per setting on a machine's first `xo test` (2.1 s and
  1.45 GB peak each on the Mac). Releases ship no prebuilt test variant
  (XO_RELEASE_STDSTORES ships the build's only); adding them is a
  release.sh change for later.
- A one-test program's edit on darwin/arm64 (one run each, same cache):
  ssa 187 to 25-32 ms, wall 0.99 to 0.20-0.38 s, peak RSS 971 to
  758-794 MB; both its tests pass, one of them through MemFs into std's
  Fs code from the store. LINUX_AB.
- A test binary linked with the variant is not byte identical to one
  built whole (the store's layout), but its tests behave the same.

## Alternatives

- Make the build's std serve tests by doing the memfs and test world
  routing at link time (a call table instead of rewritten calls): one
  variant, but a runtime indirection on every Fs call in test binaries
  and a larger change to ssa's test mode.
- Leave `xo test` whole and rely on lazy parse alone: the edit keeps
  compiling std's reachable code on every run (the 187 ms ssa above).
