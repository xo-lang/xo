# A seed build of xo on Linux needs XO_NOLIBC=0

Status: Observed (2026-11-08)

## Symptom

`$XO_SEED build --std=std -o OUT ./compiler/xo` on a linux/arm64 offload
host fails at link time with

    xo: link: pthread_setspecific is not defined (a static link with no C library)

while xobuild, with the same seed and sources, builds xo.

## Cause

On Linux, xo's default is the static link with no C library, and
compiler/xo needs the C library (pthread). xobuild sets `XO_NOLIBC=0` for
every build of xo on Linux (internal/xotool hostBackend), so a hand
written build of compiler/xo with the seed that leaves it out does not
build what xobuild builds.

## Fix

Set `XO_NOLIBC=0` on Linux for any build of compiler/xo outside xobuild,
as scripts/gate.sh's seedbuild step does (unless XO_NOLIBC is already
set).

## Negative control (2026-11-08)

The same step's gate over a98d1d71c's tree on the linux/arm64 pass/fail
host failed as it should: `xo: the inlined arm64 module does not verify
(219 errors; the first: repl.run: ... json.DecodeErr = struct ...: struct
operands do not match the fields)` from the 0.44.0 seed under
XO_INLINE_STRICT=1. It failed already in the host gate's xobuild step,
since the host had no cached ancestor build that could build that tree,
so the host gate catches the class with or without the seedbuild step;
seedbuild is what catches it when xobuild finds a cached build that
does.
