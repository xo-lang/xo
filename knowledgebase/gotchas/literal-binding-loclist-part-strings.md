# A literal binding's location list depended on the part's other functions

Status: Observed (2026-10-22), fixed the same day.

## Symptom

internal/arm64 TestBackendCacheEditIdentity failed on linux/arm64: the
build through the back-end cache differed from the cache-off build in
.debug_loclists and .debug_info (darwin passed: its DWARF is in the
dSYM, which the test does not compare).

## Cause

25ea44f6c (XO_TRACEBACK=locals) describes a binding of a string literal
by the literal's object (dwvars.xo dv_const) only when that object
exists. It asked g.strs, the part's strings, in a fresh text, and
cstr_at, the function's own, in a canonical one (the function cache's).
In a fresh build another function of the part could have made the
string, so the cache-off build described bindings the cached build did
not.

## Fix

G.fn_strs: the strings the current function's own code names (str_obj),
reset per function; dv_const describes a literal only when the function
names it, the same in both texts.
