# A conc_off stub inlined into its caller holds the caller's moved arguments at a fault

Status: Observed (2026-11-10, decision 0596)

## Symptom

After std's test variant declared its roots (decision 0596), `xo dev stdcache --test --conc=0`
under `XO_INLINE_STRICT=1` (every Go test's xo) failed:

    the inlined arm64 module does not verify (5 errors; the first: testing.fake_db: b18:
    v80 can fault while v6.0 is held 1 times (no landing releases it))

Without strict mode the whole std test variant was silently built without inlining (one line
on standard error, which stdcache does not show), so every `xo test --std-module` ran slower
std code. The 62-module `xo test --std-module` sweep passed: it does not set strict mode.

## Cause

In a std object without concurrency (`XO_STD_CONC=0`), `conc_off` (compiler/ssa build.xo)
replaces a value that needs concurrency with a fault. `sql.new_db` builds a Mutex and a Chan,
so its body is a fault that holds its owned parameter (the tables map). Its caller moved the
map into the call and placed no landing for it. Inlined (`testing.fake_db` inlines
`sql.memory`, which inlines `new_db`), the fault sits in the caller, which still holds the
map: the verifier rejects the module. The ordinary `--conc=0` variant did not inline that
chain, and before 0596 the test variant rooted nothing, so `std/testing` was never lowered
as a whole.

## Fix

`ssa.CONC_OFF_FAULT` names the fault's message; the inliner (compiler/arm64 inline.xo) treats
a callee whose summary names it as hard ("concurrency off stub"). A program without
concurrency never calls such a function, so it costs nothing as a call. INLINE_CACHE_VERSION
v13. To find which function does not verify, print every error and the inlined function's SSA
(`ssa.Module.write_func`) where compiler/xo main.xo inline_or_defer verifies.
