# A deferred removal runs before the test's parallel subtests

Status: Observed (2026-11-06)

## Symptom

Linux/arm64 host 1's /tmp filled with /tmp/xo-fexprog-* directories of
about 2.1 GB each, 7 in 2 hours, none with testtmp's owner.pid, so
testtmp.Sweep kept them for its 24 hour unowned age. internal/arm64
TestFrontCacheAcrossPrograms made them on every run, passing or not.

## Cause

The test made its work directory with testtmp.Dir and removed it with
`defer os.RemoveAll(work)`, then ran its three orders as `t.Run`
subtests that call `t.Parallel()`. A parallel subtest waits until its
parent's function returns, so the deferred removal ran first: it deleted
owner.pid and the first pass's back-end cache, and the orders then made
cache/ and out/ again, which nothing removed. The orders also never read
the first pass's entries (those of XO_TEST_WARM_XO, decision 0275), the
part of the property the first pass was there for.

## Fix

`t.Cleanup(func() { os.RemoveAll(work) })`, which runs after every
subtest, parallel ones included. In a test whose subtests call
t.Parallel directly under it, never defer the cleanup of state they
use; a parallel subtest under a serial `t.Run` group (TestModuleCheckAuto)
is safe, since the group's t.Run waits for it.
