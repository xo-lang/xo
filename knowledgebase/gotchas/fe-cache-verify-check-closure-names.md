# Front end cache verify: unwinding checks name closures by build number

Status: Observed (2026-11-12), fixed.

## Symptom

`internal/arm64 TestChainKeysSharedCache` failed deterministically on main (20382a425, Mac and linux/arm64
host 2) in its last loop, the `XO_BACKEND_CACHE=verify` rebuild of http_serve_content:

    XO_BACKEND_CACHE=verify: 1 function bodies' SSA differs from their front end cache entries, the first time.civil_instant

The cold and warm builds through the shared cache were byte for byte the cache-off builds; only verify failed.
It began with 6df938e99 (decision 0614 step 1), which gave `time.civil_instant` its first closure
(`cl`, capturing the `clamp` parameter); its parent ddd37586d passed.

## Cause

A closure is named `NAME.fnN` with N from the builder's count, so the same body's closure is
`time.civil_instant.fn2` in one program and `fn4` in another. A replay renames it (fe_replay's renames, for the
body and for the unwinding checks, `UCheck.callee`), so a hit builds the right code. Verify's comparison,
`fe_same_numbered`, numbered the closures inside the body by first use but compared the entry's checks part
(tail_split's first part: each check's value, callee and weight) as recorded, so a body that calls its closure where
an unwinding check is kept differed by the closure's number alone.

## Fix

`numbered_tail` numbers the body's closures, then goes on with the same map through the checks
(`numbered_checks`), and verify compares the two together: a check naming another closure than the body's still
differs. Test: compiler/ssa/fecache_test.xo "verify numbers the closures the unwinding checks call alike";
TestChainKeysSharedCache.

## How to avoid

Anything verify compares that holds a name the builder numbers (closures `NAME.fnN`, constants) must go through the
same numbering as the body; a replay's renames list (fe_replay) shows which parts carry such names.
