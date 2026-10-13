# Full-suite batches after switch point B (2026-10-13)

The lead's combined full runs (`XO_SELFHOST_TIER=full`, every package
but runs/) on main after switch point B (730c67a), for decision 0178
stage 7's deletion criterion "two combined full-suite batches after B
with no regression". Logs are in the lead's scratchpad.

| Batch | Commit | Where | Result |
|---|---|---|---|
| 2 | 5e25745 | EC2 r8g.12xlarge spot, in two parts (part 1 reclaimed after 27 of 44 packages) | 1 failure: cmd/xo TestNoGCTests/test_shutdown on llvm (XO-F001 deadlock), fixed in afe4aefb (two native test world bugs); does not count |
| 3 | d2c87ca6 | EC2 r8g.8xlarge on demand, i-EXAMPLE34 (terminated) | **39 packages ok, 0 failed**; internal/selfhost 3,437.9 s, cmd/xo 1,190.8 s; peak memory in use 188,684 MB; counts as clean batch 1 of 2 |
