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
