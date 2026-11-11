# The self build's `--ab` misses emit costs that grow with a module's shapes

Status: Observed (2026-11-10, Xo Lang 1, decision 0564 step 3)

Symptom: the 0564 step 3 reland passed the self build's `xo bench --ab`
(wall 1.015 loaded, phase rounds 1.003), then aiplatform/v1's cold test
build (xo-lang/gcp) emitted in 493.7 s against 43.8 s at the parent, 11x.
It was reverted the same day.

Cause: two code generation passes were quadratic in the number of word
loads in one block (arm64 cluster_mem scanned to the block's end for each
word access, parsing each instruction again; reuse_loads copied a base
register's key list at each load). Native sret loads a result word by
word, and aiplatform/v1 returns about 81 results of about 200 K words, so
its blocks held hundreds of thousands of loads. The self build has no
result past 131 K words in total, so neither pass grew there.

Fix: the two passes made linear (96a9b7414, 7fa71fa96), and results past
4,096 words kept in their slot (decision 0564's 2026-11-11 amendment). A
change to what emit does per value (lowering, loads, planning) carries a
cold aiplatform/v1 emit row beside the self build's `--ab` before it
lands, and a census of the change's new paths by size (the largest
function, block and result it meets) points at the module that will show
a superlinear cost.
