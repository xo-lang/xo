# A very wide result loaded word by word made xo's own build 14% slower

Status: Observed 2026-11-10 (decision 0564 step 3, Xo Lang 1)

## Symptom

After 0564 step 3 (native sret, 686ba3565) `xo bench --ab` of xo's self
build on linux/arm64 read wall 1.136 and cpu 1.048 worse, with output
code 0.12% smaller. Profiles showed no new hot function: regalloc, isel,
liveness and layout all a few percent slower. With XO_EMIT_JOBS=1 emit
took about 6 s longer, and with the default jobs emit's wall grew by
almost the same 5 s: the extra work sat on the parallel emit's critical
path.

## Cause

A few large driver functions (compiler/xo core.run_doc, fix_run,
run_check, ci_check, run_vet, semantic.query) doubled in size with the
same calls: a result of about 600 words that one call returns and the
next takes as an argument. sret_set loads every word of a result read
from the shared temporary area as a value of its own, so the argument
became 600 loads and 600 stores, and each word went through isel and
regalloc, where the earlier convention copied the block with q register
pairs. Those functions are in the largest emit part, so their extra time
went straight onto the build's wall clock.

## Fix

Step 3 was reverted (9ed125e41); a cap on sret's result size did not
recover the size (ADR 0564 Step 3). A redesign keeps block copies for
large results. To find such a regression: compare per-function sizes (nm
-S on both outputs) and the emit phase with XO_EMIT_JOBS=1 against the
default; a wall ratio far above the cpu ratio points at the parallel
stages' critical path.
