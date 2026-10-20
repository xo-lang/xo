# Identity of a std and compiler change together

Status: Observed (2026-10-19, the heap guard, XO_HEAP_GUARD).

## Symptom

scripts/arm64-identity.sh BASE_XO NEW_XO printed "compared 402,
differing 402" for a change meant to leave every default build unchanged
(a new llrt call that the new compiler folds to false). The binaries of
a hand built golden, each xo with its own checkout's std, were byte
identical.

## Cause

Three things, each enough alone:

- The script runs both xo binaries on the new checkout's std. The old
  compiler does not know the new fold, so it compiles the call.
- A folded `if` (compiler/ssa if_head, target_test) still makes its join
  block: the asm has one more label line, which assembles to nothing.
  Existing folds (prof.supported, target.os tests) do the same.
- Any line added to a std file moves the `FILE.xo:LINE:COL` fault
  positions of the functions after it, in rodata.

## Fix

For a change to std and the compiler together, compare each xo on its
own checkout's std, copied to one path (the directories the debug
sections name must match), the asm normalized for source positions and
label-only lines: `scripts/arm64-identity.sh --own-std BASE_XO NEW_XO`
(each xo's std is XO/../../std), which the heap guard's overlap gate used
(8a48ac13: 402 of 402 identical, arm64 and amd64). Put new llrt
functions at the end of their file, so fewer positions move. The plain
mode stays right for a compiler-only change: it compares the asm
exactly.
