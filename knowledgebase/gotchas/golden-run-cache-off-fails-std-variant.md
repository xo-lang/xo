# TestGoldenRun under XO_BACKEND_CACHE=off fails every golden at once

Status: Observed (2026-11-07, Xo Lang 4)

## Symptom

`XO_BACKEND_CACHE=off go test ./internal/arm64 -run '^TestGoldenRun$'`
failed all 633 goldens in under 0.2 s each with `build: xotool: std's
variant conc=0 did not build ... cache std off (XO_BACKEND_CACHE=off):
the whole program is built`. It looked like a compiler bug in the change
under test (a lazy landings prototype) and cost a host 2 run.

## Cause

The goldens build separately compiled (xotool builds std's variants
first, XO_TEST_SEPCOMP), and std's variants live in the back-end cache;
with the cache off the variant build exits non-zero, so every golden's
build fails before compiling the program.

## Fix

Do not set XO_BACKEND_CACHE=off on a golden run to force a cold build:
use a private XO_BACKEND_CACHE_DIR instead, or XO_TEST_SEPCOMP=0 for whole
program builds. Read the first failure's message before treating a
fail-everything run as the change's (a build error at 0.00 s is the
harness, not the program).
