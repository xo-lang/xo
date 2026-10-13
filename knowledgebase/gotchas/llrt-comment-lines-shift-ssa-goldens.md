# A comment edit in std/native/llrt shifts the SSA goldens

Status: Observed (2026-10-13, decision 0198).

## Symptom

Rewording the header comment of `std/native/llrt/num.xo` (four lines
to three, no code change) failed `go test -short ./internal/ssa`: the
SSA dumps under `internal/ssa/testdata/bench/*.ssa` and
`internal/ssa/testdata/golden/*/want.rc.ssa` print positions such as
`@ num.xo:65:20`, and every one after the edit moved up a line.

## Cause

The runtime modules in `std/native/llrt/` (and `std/native/prelude.xo`)
are compiled with every native program, so their functions appear in
the SSA goldens with source positions. Adding or removing a line
anywhere above a function changes its positions in every dump that
includes it.

## Fix

Keep the line count of a comment the same when only the wording
changes (reflow the text into the same number of lines), or update the
goldens with `go test ./internal/ssa -update` and review that
the diff is positions only. Check with `git diff --numstat` on the
llrt files: equal added and removed counts mean no shift. Deleting
whole declarations from `std/native/rt.xo` or `rtjson.xo` (bodiless
signatures) shifted nothing.
