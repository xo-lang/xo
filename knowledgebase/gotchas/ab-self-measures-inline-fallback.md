# A self --ab can time a build that fell back to no inlining

Status: Observed (2026-11-03, Xo Lang 1, decision 0465 step 2)

## Symptom

`ab-host.sh BASE NEW NAME -- self` reported NEW 6% faster in wall time
(0.94), 10% less cpu and 11.5% less memory than BASE. The change under test was a
lookup in reference counting and was expected to save well under 1%. The
same tree then failed `XO_INLINE_STRICT=1 go run ./scripts/xobuild` with
`the inlined arm64 module does not verify (486 errors ...)`.

## Cause

Without XO_INLINE_STRICT=1, an inlined module that does not verify is
only a warning: xo builds the program without inlining
(compiler/xo main.xo inline_verified). That skips the inliner's work on
the module, so the bug made NEW's builds faster and smaller. The --ab
timed NEW's xo building compiler/xo, every round on that fallback path,
and printed no error. Here the verifier's own checker (ssa verify.xo
rc_checker) lacked a table the new code read, so it rejected every view
call.

## Fix

Before trusting a compile-time --ab, build NEW once with
XO_INLINE_STRICT=1 (xobuild with it set, or the tree's go tests, which set
it through internal/xotool), or grep the run's stderr for `built without
inlining`. Distrust a large unexpected speedup in a change that should
cost or save little. An Rc built outside rc_prepare (the verifier's
rc_checker) must set every field the counting reads.
