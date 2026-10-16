# The arm64 function cache key left out the source lines

Status: Observed (2026-10-16, found by reading fcache.xo fn_key while
adding inlined frames, decision 0239; no wrong build seen).

Symptom (expected): after an edit that only moves code (a comment added
above a function), a build with the function cache reuses the
function's entry, whose debug line rows name the old lines.

Cause: fn_key hashed each value's operation, operands, block, and fault
position, but not Value.line (added for the line tables), nor the
function's file or inline sites; a function's code is the same, its
rows are not.

Fix: with debug information on, the key holds the file, the inline
sites, and each value's line and site (FN_CACHE_VERSION v3). A new
input of the emitted text or its records must go into fn_key too.
