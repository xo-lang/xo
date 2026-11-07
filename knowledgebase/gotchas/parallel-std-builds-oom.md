# The arm64 suite's parallel std builds ran out of memory

Status: Observed (2026-11-06)

## Symptom

`go test ./internal/arm64` (full) on linux/arm64 host 2 failed tests of
the separate compilation and std record paths (TestSepcompStdxKeepsFrontCache,
TestStdRecordIdentity, TestStdTreesRebuild, TestStdRecordBindRetry,
TestSepcompDefault, TestTracebackSeparate sep=true) with `xo dev
stdcache: exit status 1` or `cache std ... failed (xo dev stdobj exit
137)`. Each passed run alone, and which ones failed changed from run to
run (main failed TestSepcompBackgroundBoth and
TestFirstEditAfterColdParts the same day).

## Cause

Exit 137 is SIGKILL: the container's memory cgroup (memory.max 22 GB)
reached its limit while the suite ran (memory.current 21.96 GB,
memory.events oom_kill rose by 5 in a run of main fdd0628a3 that
failed 5 such tests). About twenty parallel tests each
start `xo dev stdcache`, whose `xo dev stdobj` child compiles std
(1.39 GB peak each, cold, linux/arm64), beside the package's other builds, and nothing
limited how many ran at once. dmesg is not readable in the container;
/sys/fs/cgroup/memory.events is.

## Fix

stdslot_test.go's stdRun runs each explicit std build in one of a few
slots (half the memory limit over one build's peak; XO_TEST_STD_BUILDS
sets it). Background std builds a build starts itself
(XO_SEPCOMP_BG) are not counted. To check for an OOM kill on a Linux
host, read memory.events' oom_kill before and after a run.
