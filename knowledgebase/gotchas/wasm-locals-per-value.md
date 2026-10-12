# One set of wasm locals per SSA value explodes on large structs

Status: Observed (2026-10-12), selfhost/wasm stage 2 (decision 0179).

**Symptom.** selfhost/wasm built selfhost/check (the self hosted
checker) into a module of 1.67 GB (WAT 8.85 GB, 693 s and 14.5 GB of
memory to build) that no engine loads: 162 functions had more than
50,000 locals, V8's limit, one of them 3,548,286. The goldens and the
self hosted parser (23,135 locals at most) were fine, so nothing failed
until a program with a large struct was compiled.

**Cause.** Values are their leaves in locals (decision 0179, "Values"),
and stage 1 gave every SSA value fresh locals. The checker passes its
`Checker` struct (253 leaves) as a `var` parameter and updates its
fields everywhere: every update and every call result is a new value,
so a long method made thousands of 253 local values, and copied all of
them for each field update.

**Fix.** `plan_locals` (selfhost/wasm/func.xo) shares locals between
values whose live ranges do not overlap (SSA liveness, a linear scan),
updates a dying struct's field in place, and lets a field read use its
struct's locals. Two traps on the way: a block parameter is written at
the end of each predecessor (its range must cover those), and the
blocks are not in dominance order, so a chain of field reads must be
resolved recursively, not in block order (the golden nested_update read
a stale map when it was). Check the largest function's locals
(`grep -F "  (func \$f" x.wat`) when a big program is first built.
