# A size measured with the backend cache on depends on the cache

Status: Observed 2026-11-10 (decision 0564 step 3, Xo Lang 1)

## Symptom

Two xo binaries with byte-identical backend code (kh's parent 0699743b2
and its revert 9ed125e41) built the same source (compiler/xo) to __text
41,189,732 and 40,377,272 bytes with the shared backend cache on. The
difference read as kh making xo 1.76% larger (ADR 0564 Step 3, corrected
the same day).

## Cause

With the cache on, a build reuses cached work its own compiler did not
produce in this run, and the output's size changed by 2% with what the
cache held. With XO_BACKEND_CACHE=off both compilers give 41,189,732,
twice each.

## Fix

Measure code size (`size -m`, `nm -S` diffs) with XO_BACKEND_CACHE=off,
or with a private, empty XO_BACKEND_CACHE_DIR per compiler. A cached build
that differs from a cold build of the same source is itself a bug to
report (the cache should not change the output).
