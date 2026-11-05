# A separately compiled program's link fails with a duplicate `_xo_f_zero__val_...`, only when its build order makes its own copy

Status: Observed (2026-11-05, xo-lang-4, aws issue 12).

## Symptom

A native build of an S3 program in the AWS SDK failed to link with
`duplicate symbol _xo_f_zero__val_2enative_2ehttp_2eRequest in
prog-darwin.15.o and .../w-.../prog-darwin.3.o`. It reproduced only
with the user's warm cache: with a fresh `XO_BACKEND_CACHE_DIR` it
linked (the first builds are whole-program, see
sepcomp-default-check-needs-std-variants.md), with
`XO_BACKEND_CACHE=off` it linked, and after one more build in the same
cache it linked. Small programs that return `http.Request` from a
faulting function never reproduced it, because they took std's copy.

## Cause

Since 2d51a8403 (2026-10-30), a fault exit zeroes a struct result of 8
or more fields by calling a shared `zero_val.T` (ssa build.xo
zero_val_of). The function's file is that of the first function whose
build made it. When a program function's build makes it before any std
function's, the program emits its own copy, and std's object (decision
0388) has one too. arm64 gen.xo `shared_name`, which decides what is
weak under `Module.weak_shared`, listed `zero.` but not `zero_val.`, so
both copies were strong. Std's conc=false object held 68 strong
zero_val definitions.

## Fix

`shared_name` includes `zero_val.`, so both copies are weak and the
link keeps one. internal/arm64 TestSepcompZeroValWeak builds std's
object and fails on any external `zero.T` or `zero_val.T` text
definition that is not weak. It does not depend on which build makes
the program's copy. A generated, shared helper with a new name prefix
belongs in `shared_name` from the start.
