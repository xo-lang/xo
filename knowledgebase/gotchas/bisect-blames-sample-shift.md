# A bisect of a sampled test blames the commit that moved the sample

Status: Observed 2026-11-21 (alert 107195, task #587).

## Symptom

The main-failure bisect named 54f87386a as the first bad commit for
TestModuleCheckAuto/grpc_calls. That commit only added a golden.

## Cause

TestModuleCheckAuto (internal/arm64) runs every 8th golden unless
XO_SELFHOST_TIER=full. The added golden moved the sample onto
grpc_calls, which had been broken since 19998d4fc but never ran. The
bisect ran the top-level test (`--from-log` cut the subtest), which
passed at every commit before the shift.

## Fix

mainbisect.sh keeps the failed subtest; mainbisect-remote.sh runs it
with a per-level anchored `-run` and `-v`, counts a run only when the
subtest's `--- PASS`/`--- FAIL` line is there, checks it at GOOD first and
pins it with XO_SELFHOST_TIER=full when it did not run there. A bisect
point where it does not run is dropped. mainbisect_test.sh's sample
shift case covers it. A new sampled test must take XO_SELFHOST_TIER=full
to run every case, or the pin finds nothing (MAINBISECT-NOTRUN-PINNED).
