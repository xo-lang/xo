# A test that ends its process ran again without --check

Status: Observed (Xo Lang 1, 2026-11-18, task #354)

## Symptom

`xo test --check=free` of std/native/llrt/heapblk passed its plant (a
free by the block's prefix, which ends the program with status 70 under
the free check), and an `expect(checking()) == true` in it failed, while
the same expect in a test that did not end its process passed. The SSA
and the assembly were right. Six probes and a sweep of `XO_PASSES=-name`
over every optional pass went to a miscompile that was not there.

## Cause

compiler/xo/test.xo run_batched runs a batch's tests in one process; a
test whose process ends in it (an exit, a fault) runs again alone so its
report is the one process mode's. That rerun passed `[]` as the
environment, so XO_FREE_CHECK, XO_LEAKCHECK, XO_PROF and XO_TRACE were
gone and the test ran without the check it had just tripped. Property
tests' cases, shrinking replays and their serving binary never got the
--check words either.

## Fix

The alone rerun gets `prof_words(o)` as the batch did, and the property
paths take `check_words` (`cw`). internal/arm64 TestCheckFlagPlants's
heapblk row fails without the fix. When a check flag's plant passes, run
the test with the variable set by hand (`XO_FREE_CHECK=1 xo test ...`)
before suspecting the compiler: a pass there means the harness dropped it.
