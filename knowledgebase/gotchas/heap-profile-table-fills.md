# A heap profile's sample table fills, then later allocations are not in it

Status: Observed (2026-11-01)

Symptom: `XO_PROF_HEAP_AT` late in a self build shows 244 MB in use of 3.3 GB,
and "allocated" stops at 263 GB.

Cause: 65,536 samples at `XO_PROF_HEAP_RATE=4194304` (4 MiB) fill at 262 GB;
nothing after that is sampled.

Fix: the rate times 65,536 must exceed total allocation (about 290 GB for a self
build): 16 MiB, `XO_PROF_HEAP_RATE=16777216`. Compare `allocated` with the
expected total before believing a profile.
