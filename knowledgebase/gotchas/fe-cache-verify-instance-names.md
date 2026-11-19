# Front end cache verify: generic instances compared by name

Status: Observed (2026-11-18), fixed.

## Symptom

Building any of 18 http goldens, then http_h2c, with one cache and `XO_BACKEND_CACHE=verify` failed:

    XO_BACKEND_CACHE=verify: 1 function bodies' SSA differs from their front end cache entries, the first http.dispatch

The entries' unwinding checks named `prelude.list_sort[Str]` in one build and `prelude.list_sort[Str]#2` in the
other. Seen on main (a24bae636) too.

## Cause

A generic instance gets `#n` after its name when another function of that name was made first, so its name
depends on what the program made before. An entry lists each instance it calls with its key and its name
(fe_greads), and a replay renames by the key, so a hit builds the right code. Verify's comparison
(`fe_same_numbered`) compared the names as recorded.

## Fix

`entry_instances` reads each entry's instance list, and `numbered_tail` seeds its map with each name as
`generic KEY`, so the checks and the body name an instance by its key (closures are still numbered by first use,
`closures_in`). Tests: compiler/ssa/fecache_test.xo "verify names the generic instances the checks call by key";
internal/arm64 TestFrontCacheVerifyAcrossPrograms/http_h2c.

## How to avoid

As fe-cache-verify-check-closure-names.md: a name the build numbers (closures, `#n` instances) goes through the
same renaming verify uses as the replay does.
