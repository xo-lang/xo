# Two full internal/arm64 suites on one host fail on memory

Status: Observed (2026-11-04, Xo Lang 1, linux/arm64 offload host 2).

## Symptom

A full `go test ./internal/arm64` run failed 13 tests at once: the sepcomp
links, the std record and store builds, TestPDBGUIDFromContent,
TestSwitch65535Arms and both TestStrJoinPast4GiB and TestStrViewPast2GiB
(the last with `exit -1`, the program killed). It took 1841 s against
about 900 s alone. Rerun alone, all 13 pass on the same commit.

## Cause

A second full suite (another branch's gate, same agent) ran on the same
24 GB host at the same time. The two big string tests each allocate 2.2 to
4 GB beside the suite's parallel builds; with two suites the host ran out
of memory and the kernel killed programs. Even with only the 13 failing
tests run together, TestStrViewPast2GiB failed again (12 GB available);
alone it passed in 3 s with 19 GB available.

## Fix

Run one full internal/arm64 suite per host at a time: chain a second
branch's suite behind the first (wait for its GATE-DONE) instead of using
the agent's second lease for it. A failure set made of the memory heavy
tests, with a doubled run time, is a contention signal: rerun those tests
alone before suspecting the change.
