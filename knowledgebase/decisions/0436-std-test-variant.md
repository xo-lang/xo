# 0436: std's separately compiled variant for test builds

Status: Accepted (owner, 2026-10-30; was Proposed (xo-lang-4, 2026-10-30; the lead approved building it, the owner accepts with the measurements below))

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
program's concurrency, as build does.

`--std-module=M` (item 2 of the test speed queue, 2026-10-30) takes the
variant too, with M and every std module that imports it, directly or
not (std_source_set, from std/native's `use` lines), built from source
(check.Checker.sep_source: ssa's run_job never makes their functions
extern): their store code may hold M's old bodies inlined and its old
layouts. An importer the program never loads is dead in the link. The
variant's key leaves out std's `_test.xo` files, so a test edit keeps
it. A source edit of M changes the key; a built test variant keeps
what its key hashed in `<key>.tree`, and std_variant_near takes one
whose tree lines equal the current ones once the source set's files
are left out on both sides (the rest of std is the same files, and none
of it imports M). An edit outside that set (std/url's interface module,
another module) misses as before.

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
  Fs code from the store. On linux/arm64 (the offload host, run alone,
  `xo bench --ab` n=20 of a one-test program's test edit, A whole
  program, B the variant): wall 0.499 (264 to 132 ms, 95% interval 0.447
  to 0.558), cpu 0.397, peak RSS 0.769 (489 to 376 MB); a store of
  11.3 MB per concurrency setting there, built in 2.0 s at 1.5 GB peak.
- `--std-module` after a code edit of the module (darwin/arm64, warm,
  near variant against the whole program): base64 0.25 against 0.40 s,
  peak RSS 324 against 570 MB; url 0.29 against 0.54 s, 314 against
  582 MB; json 0.21 against 0.45 s, 350 against 614 MB. On linux/arm64
  (the offload host, run alone, `xo bench --ab` n=20 of a source edit,
  A whole program, B the near variant; the module file restored between
  modules): base64 wall 0.58, cpu 0.513, peak RSS 0.617 (13 modules from
  source); url wall 0.512 (557 to 285 ms, 95% interval 0.492 to 0.529),
  cpu 0.444, peak RSS 0.587 (654 to 384 MB; 9 from source); json wall
  0.406 (463 to 188 ms, 0.378 to 0.434), cpu 0.383, peak RSS 0.6 (692
  to 415 MB; 18 from source). Every std
  module with tests passes from the near variant after a touch of its
  source, and a layout edit (two fields prepended to url's Values)
  passes 66 of 66. std's store defines no weak external, so no
  instance from the store can stand in for the program's.
- A test binary linked with the variant is not byte identical to one
  built whole (the store's layout), but its tests behave the same.

## Alternatives

- Make the build's std serve tests by doing the memfs and test world
  routing at link time (a call table instead of rewritten calls): one
  variant, but a runtime indirection on every Fs call in test binaries
  and a larger change to ssa's test mode.
- Leave `xo test` whole and rely on lazy parse alone: the edit keeps
  compiling std's reachable code on every run (the 187 ms ssa above).
