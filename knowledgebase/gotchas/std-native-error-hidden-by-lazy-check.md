# A std/native error that only `xo dev stdobj` reports

Status: Observed (2026-10-29, Xo Lang 2's 48da2bb2b, found by xo-lang-6)

## Symptom

A std/native change passes every golden on arm64, LLVM and Go, yet
`xo dev stdobj OUT` fails with a checker error in the changed file
(here `XO0510: use of moved value` in std/native/http.xo). Default
builds on main still work, but print or log `std variant: miss` and fall
back to the whole-program build, so separate compilation (decision 0388)
silently stops being used and the next sepcomp change is blocked.

## Cause

A whole-program build checks the bodies of std functions the program
reaches, lazily; a golden that never calls the function (or calls it
through a path the lazy check skips) never checks it. The std store
build (`xo dev stdobj`, the stdcache's background build) checks every
std body in full and rejects the error.

## Fix

Run `./bin/xo dev stdobj OUT` (output in scratch, deleted after) in the
gate of every std/native change, besides the goldens; or
TestStdRecordIdentity / TestStdTreesRebuild. For move only values (a
compress Decompressor, builders) in a loop, move them in a helper
outside the loop (std/native/http.xo gz_step).
