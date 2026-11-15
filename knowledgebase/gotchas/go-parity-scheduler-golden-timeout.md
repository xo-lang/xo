# A scheduler golden timed out on the Go backend

Status: Observed (2026-11-15)

## Symptom

The continuous tier on 572a47416 failed internal/llvm
TestGoldenGoParity/fair_pairs: "still running after 2m0s", no output. The
LLVM runs of the same golden passed in 21 s.

## Cause

fair_pairs (decision 0394) makes 96M channel round trips to load xo's
native scheduler. Go's channels are far slower at it: on host 3, alone,
the Go backend's binary took 42.6 s (52.6 s on 4 CPUs) against 2.2 s for
LLVM; on the Mac 25 s against 1.6 s. Inside the tier's llvm suite on a
shared host it passed the 2 minute program limit. Not a hang, and not a
fairness failure: Go's scheduler is not what the golden tests.

## Fix

internal/llvm goParitySkip leaves fair_pairs out of TestGoldenGoParity with
its reason; TestGoParitySkipNamed requires each entry to be a golden with
a reason. A new golden that makes a backend work for tens of seconds
belongs in that list for the slow backend, or gets fewer rounds there,
rather than a longer program limit.
