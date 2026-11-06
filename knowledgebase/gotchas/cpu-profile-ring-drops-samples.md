# A CPU profile that fills its ring keeps only the run's start

Status: Observed (Xo Lang 1, 2026-11-05)

## Symptom

A cold self build profiled with `XO_PROF=cpu XO_PROF_HZ=1000` put insert_rc's
borrow inference at about 35% of the build's CPU. A second profile of the
same build at 100 Hz put all of insert_rc at 27% and borrow inference
(borrow_scan) at 2.7%. The first profile's .txt header ended with
"247476 lost", which nobody read.

## Cause

The runtime keeps CPU samples in a fixed ring (std/native/llrt/prof.xo
CAP). When the ring is full, later samples are counted as lost and
dropped, so the profile is the run's first part only: 32,768 of 280,244
samples there, the build's early phases over-weighted. `xo run
--profile` and `xo test --profile` set XO_PROF_QUIET, which also hid the
runtime's exit note about the drop.

## Fix

Since 2026-11-05 the ring holds 262,144 samples (136 MB mapped only when
profiling, touched as samples arrive). A profile that still fills it says
PROFILE INCOMPLETE in the runtime's exit note (even when quiet), on `xo
prof`'s stderr and in the .txt, with the share lost and the XO_PROF_HZ
that would have kept every sample. Before then, or for a longer run:
profile at 100 Hz (the default) or lower, check the .txt's first line for
"lost", and never rank functions from a profile that lost samples.
