# The full tier turns separate compilation off for every xo a test starts

Status: Observed (2026-11-07, tier 57).

## Symptom

Tier 57's arm64 step failed seven tests of the separately compiled
path (TestSepcompTestVariant, TestSepcompStdModuleEdits,
TestSepcompGoldens, TestStdRecordBindRetry, TestSepcompBackgroundBoth,
TestStdFailedMarkerPerTree, TestSepcompStdxKeepsFrontCache), each saying
a build did not take or load std's variants or record, e.g.
`sepgolden_test.go:58: a build did not take std's variants (no
"cache std KEY conc=false reused"): <nil>`. Each passed alone.

## Cause

The tier runs with `XO_TEST_SEPCOMP=0` (scripts/lead/tier.sh,
offload-tier.sh). Since 0bd9f22b7 (2026-11-06) xotool's init
(tierSepcomp, internal/xotool/xotool.go) then sets `XO_SEPCOMP=0` in the
test process, so every xo a test starts with `os.Environ()` builds whole
program. A test of the separate path that does not skip in that mode can
only fail there. The `<nil>` is the hint: no build failed; separate
compilation was off.

## Fix

Such a test calls `skipWholeTier(t)` (internal/arm64/debugsep_test.go),
or `sepDarwinEnv(t)` for a darwin debug test. The tier runs these tests
in their own step with separate compilation on (tier.sh SEPTESTS): the
hourly -short skips them as well, so without that step nothing regular
ran them. A new test of the separate path joins both.

A failed std variant build (`xo dev stdcache --conc=N`) is now every
later xotool.Build's error instead of a stderr line and whole program
builds, so a full suite flake from it names its cause.
