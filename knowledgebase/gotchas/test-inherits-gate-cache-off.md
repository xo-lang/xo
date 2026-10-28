# A cache test passes alone and fails in the full tier

Status: Observed (2026-10-28, lead tier 24)

## Symptom

internal/arm64 TestWindowsPDBObjCheck and TestWindowsPDBPartKeys passed in
their author's gate and alone on main, and failed in the lead's full tier
with "no PDB parts in the back-end cache (open .../cache/pdb: no such file
or directory)".

## Cause

scripts/gate.sh exports `XO_BACKEND_CACHE=off` for every package it runs
(so the goldens are built cold). A test that builds through `os.Environ()`
inherits it; setting only `XO_BACKEND_CACHE_DIR` does not turn the cache
back on, so the back-end cache, and everything stored in it (here the PDB
parts), is off in the tier and on everywhere else.

## Fix

A test that depends on the back-end cache sets `XO_BACKEND_CACHE=on` on
every build it runs, beside `XO_BACKEND_CACHE_DIR`, as the other
fcache_test.go tests do. Reproduce the tier for one test by exporting
`XO_BACKEND_CACHE=off` before `go test -run`.

Hit again the same day by internal/arm64 TestStdFactsVerify ("XO_STD_FACTS_DUMP=... is not
written by this build": the facts dump needs the cache's reference counting path). Guard
(2026-10-28): internal/repotest TestBackendCacheEnvExplicit fails when a Go test function
sets `XO_BACKEND_CACHE_DIR` in a build's environment without `XO_BACKEND_CACHE=on` or
`off` in the same function; the four other functions it found (TestBuildSweepsStaleAsm,
TestStdModuleRunCache, and two inline_test.go builds) now set it on.
