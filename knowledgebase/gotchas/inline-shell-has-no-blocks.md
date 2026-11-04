# A warm build's cached function has no blocks when the inliner reads it

Status: Observed (2026-11-03, Xo Lang 1, decision 0465 step 2)

## Symptom

`the inlined arm64 module does not verify (... edge to bN passes vM, which
is not held)` only on the build after an edit (TestStdModuleRunCache,
TestModuleCheckAuto, the front cache tests), on linux/arm64 with
XO_WHOLE_BUILD=off; the cold build and a third build passed, and a
self build under XO_INLINE_STRICT=1 passed.

## Cause

compiler/arm64/inline.xo refused a function that returns a view (ret_view)
when it had more than one Return, counting them in the module's copy of
the function. On a warm build a function whose result came from the cache
is a shell there: no blocks, so no Return. path.ext (four returns) counted
0, passed the test, and was inlined from its stored body through a join
parameter, which the counting owns, with the uncounted view.

## Fix

Read the count from the inliner's summary (compiler/arm64 inlsum.xo
FnSum.rets, inline sum v3), which a warm build reads from the cache with
the function: the same answer cold and warm. Treating "no blocks" as "not
one Return" (`!= 1`) only moved the failure: the warm build then refused
what the cold build inlined, and TestModuleCheckAuto's identity of warm and
cache-off builds failed for 16 programs. Anything the inliner's cost pass
reads from a function's blocks belongs in FnSum. To reproduce: build once,
append a comment to the program's or std module's test file, build again
(XO_BACKEND_CACHE=on, XO_WHOLE_BUILD=off, XO_SEPCOMP_BG=0,
XO_INLINE_STRICT=1), and compare XO_INLINE_REPORT=1 before and after the
edit.
