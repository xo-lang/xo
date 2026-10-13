# Full-suite batches after switch point B (2026-10-13)

The lead's combined full runs (`XO_SELFHOST_TIER=full`, every package
but runs/) on main after switch point B (730c67a), for decision 0178
stage 7's deletion criterion "two combined full-suite batches after B
with no regression". Logs are in the lead's scratchpad.

| Batch | Commit | Where | Result |
|---|---|---|---|
| 2 | 5e25745 | EC2 r8g.12xlarge spot, in two parts (part 1 reclaimed after 27 of 44 packages) | 1 failure: cmd/xo TestNoGCTests/test_shutdown on llvm (XO-F001 deadlock), fixed in afe4aefb (two native test world bugs); does not count |
| 3 | d2c87ca6 | EC2 r8g.8xlarge on demand, i-EXAMPLE34 (terminated) | **39 packages ok, 0 failed**; internal/selfhost 3,437.9 s, cmd/xo 1,190.8 s; peak memory in use 188,684 MB; counts as clean batch 1 of 2 |
| 4 | d4a8b548 | EC2 r8g.8xlarge on demand, i-EXAMPLE45 (terminated) | 38 ok, 1 failed: internal/gen TestHybridPreemption (stop the world 14 to 23 ms against a 10 ms bound under load; alone at the same commit 5 of 5 pass, gotchas/hybrid-preemption-stw-under-load.md). TestSelfhostWasm was skipped (no Node on the instance, gotchas/ec2-full-tier-skipped-wasm.md) while main had a wasm regression (fixed in 0c5040eb). Does not count |

Batch 3 also ran without Node (TestSelfhostWasm skipped); the wasm tier
passed at that commit in the agents' targeted runs, so batch 3 still
counts, but batch 5 is the first batch that runs the wasm tier on EC2.
| 5 | 7cd80f04 | EC2 r8g.8xlarge on demand, i-EXAMPLE14 (terminated), node v22.22.1 (the wasm tier ran) | 38 ok, 1 failed: internal/gen TestHybridPreemption again (the load flake of batch 4, gotchas/hybrid-preemption-stw-under-load.md); every other package including internal/selfhost with the wasm tier passed. Not counted until the flake is fixed |
