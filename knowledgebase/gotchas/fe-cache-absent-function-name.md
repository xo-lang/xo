# A cached body found a function name free that is now another function's

Status: Observed (2026-10-20, decision 0271 step 1a, 2e0f5dce; fixed
the same day).

## Symptom

After an edit, a build through the front end cache wrote a binary that
differed from a build with XO_BACKEND_CACHE=off, and
XO_BACKEND_CACHE=verify named the body (`fb`). The program printed the
right values by luck. Step 2's prebuild (bodies built in other builders)
showed it in bulk: 8,050 functions of xo's asm differed and the fixed
point failed.

## Cause

Step 1a's commit checked what a body found (it must still be there) and
replayed what it made, skipping a type or helper of a name already in
the module. That is right for types, whose names say what they are, but
not for functions: the builder chooses a helper's name by whether the
name is free (map_remove adds a result suffix, helper_name tries `#2`).
A Map[Str, Bytes] remove recorded alone named its helper
xo.map.remove[Map[Str, Str]]; committed after a Map[Str, Str] body had
made that name for another result type, it called the other helper.

## Fix

An entry also holds the function names the body looked for and did not
find, and the helpers it made, and a commit requires them absent (a
miss otherwise). Anything a builder decides from absence must be
recorded as well as what it found. Test: internal/arm64
TestFrontCacheAbsentHelper.
