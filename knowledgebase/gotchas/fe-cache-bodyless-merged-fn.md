# XO1201 "a function without a body" on a merged std module's fast path

Status: Observed (2026-11-11, Lang 3, while merging std/regexp)

## Symptom

A native build (seen on arm64) fails with

    error XO1201: not available on the native backends (arm64, amd64, LLVM) yet: a function without a body (support is planned)

for a program that calls a merged std module's bodyless function (decision
0597 part 2: std/html's `escape_string`, std/regexp's `must`; std/url's
`escape` is private), and `go run ./scripts/xobuild` of compiler/xo fails
the same way with the earlier bin/xo and the seed. The same build with
`XO_BACKEND_CACHE=off` or `XO_FE_CACHE=off` works, and so does one with a
fresh private `XO_BACKEND_CACHE_DIR`.

## Cause

A front end cache entry (compiler/ssa/fecache.xo) of a caller's body,
recorded while the callee's module was not merged, declares the std
function itself: an unmerged module's declaration was its whole
implementation. Merging keeps the declaration's interface text (the
declaration file was bodyless already), so the caller's key did not
change, and the replay declared the now bodyless function, whose body
SSA cannot build. A correct recording declares the native part
(std/native/M_native.xo) instead. Which xo wrote the stale entry was not
found: a deliberate repro (entries recorded by an xo with std/regexp
unmerged, replayed by the new one) did not fail, since its std sum
differed.

## Fix

fecache.xo's replay treats a declare event of a native part (build.xo
`native_part`) as a miss ("native part"), so the body builds from source.
contract.xo's XO1201 text now names the function.

Workaround with an xo older than the fix: build once with
`XO_FE_CACHE=off` (the entries are in the cache's packs, not one file to
delete); that build's entries replace the stale ones. Rebuild compiler/xo
with `XO_FE_CACHE=off go run ./scripts/xobuild` when it is the build that
fails.
