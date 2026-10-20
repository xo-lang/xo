# A body's one-way flag set was hidden by the builder that recorded it

Status: Observed (2026-10-20, decision 0271 step 2's prebuild; fixed in
the entry format the same day).

## Symptom

Bodies recorded by step 2's prebuild (other builders, any order)
committed without a one-way flag the serial build would have set: an
LLVM release build failed on an undefined @xo_x_chan_free (the network's
C ABI exports were not declared). A serial build through the cache did
not show it: there the commit's declares put the network's modules in
decl_mods, and the task scan sets need_conc before any body is built.

## Cause

need_conc and uses_net are one-way flags: once a body sets one it stays
set, and the build reads it at the end (need_conc rebuilds the program
with unwinding checks; uses_net declares the network's exports). The
entry kept no record of a set: fe_fixed only saw a body change them, so
a body recorded by a builder that had the flag set already looked as if
it set nothing.

## Fix

Bodies set the flags through Builder.set_flag, which notes the set while
a body records; the entry carries the bits and the commit sets them
again (FE_CACHE_VERSION v4). Any state a body can only add to must be
recorded as what the body did, not as a change the recording builder
saw. Test (a guard of the format): internal/arm64
TestFrontCacheOneWayFlags.
