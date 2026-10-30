# XO_DUMP_FN writes nothing when the run cache serves the build, and `xo dev dump ssa` is the SSA before inlining

Status: Observed (2026-10-30, Xo Lang 1, the parse_duration branch work).

## Symptom

`XO_DUMP_FN=text.parse_duration XO_BACKEND_CACHE=off xo build .` printed
only `darwin/arm64: PATH`: no `the SSA and MIR of ...` line, no
xo-ir-*.txt, and no "no function of that name reached the backend"
warning. Separately, `xo dev dump ssa .` showed `v25 = not v21; jump
b8(v25)` with plain_end a call, which led to an isel fix for a shape
the backend never sees (after inlining the Not reads a join parameter).

## Cause

The whole-build run cache (decision 0360, XO_WHOLE_BUILD) served the
unchanged program's binary before the backend ran; XO_BACKEND_CACHE=off
does not turn it off. `xo dev dump ssa` dumps the module as built, before
the native inliner and the reference counting.

## Fix

Dump with `XO_WHOLE_BUILD=off XO_DUMP_DIR=DIR XO_DUMP_FN=NAME xo build`;
its xo-ir-NAME.txt holds the SSA after inlining and the final MIR. Use it,
not `xo dev dump ssa`, to see what the native backend lowers.
