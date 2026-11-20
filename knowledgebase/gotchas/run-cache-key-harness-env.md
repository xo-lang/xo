# A test harness's own XO_ variable splits xo's run cache key

Status: Observed 2026-11-19 (task #425, decision 0657 stage 2).

## Symptom

The gate's binary selection builds each LLVM golden
(TestGoldenBinSelectBuild), then TestGolden builds the selected goldens
again in the same gate, and the second build misses xo's binary cache:
54 cache files after the selection, 105 after TestGolden for the same
goldens, so the head side of the selection was paid twice.

## Cause

xo hashes every `XO_` variable into the run cache key (decision 0360,
compiler/xo/testcache.xo run_key_of), except XO_TEST_JOBS, XO_TEST_ONLY
and XO_BUILD_DIR. The selection runs with XO_GOLDEN_BIN_OUT set and
TestGolden with XO_LLVM_GOLDENS set; neither is read by xo, but each
made a different key. internal/xotool.Build passed the whole test
environment on to xo.

## Fix

internal/xotool.Build drops the variables only the Go tests read
(`harnessEnv`: XO_GOLDEN_BIN_OUT, XO_LLVM_GOLDENS, XO_ARM64_GOLDENS)
before starting xo. A new harness-only `XO_` variable that a golden
build may see goes into that list, or it splits the cache the same way.
Test: internal/llvm TestGoldenBinSelectSharesCache.
