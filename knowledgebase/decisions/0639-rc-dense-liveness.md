# 0639: Rc liveness on dense leaf bit sets

Status: Accepted (lead, 2026-11-14: cut 3 of the 2026-11-14 re-profile; push after its --ab).
Compiler internal: no language or output change.

## Context

The 2026-11-14 re-profile (~/xo-lead/notes/lang1/retime-rank-2026-11-14.md) put insert_rc.apply
at 9.6 to 12.8 s of the self build's CPU, the largest phase after the inliner moved to the task
pool (0636). Its flat top was Rc.liveness (7.6 s self, 20.9 s cumulative) and the map puts under
it: live_in was a List[Map[Int, Leaf]] and live_out a List[Set[Int]] keyed by leaf_key
(root * 2^31 + path id), and every block's live set was sorted by (root, path rank) again for
EdgeNeed.

## Decision

compiler/ssa/rc.xo numbers the leaves a function tracks once, in the (root, path rank) order
sorted_leaves gives, and runs liveness on bit sets over those numbers:

1. One pass collects every tracked leaf the function uses, sorts the table once and gives each
   leaf its position as its id (RcFunc.lid maps a key to it).
2. live_in and live_out are bit sets (64 leaves per word, one row per block, in RcFunc.live_bits);
   the worklist joins and kills whole words.
3. A block's sorted live list is its set bits in ascending order, so the per block sort is gone.
4. EdgeNeed carries the bits and the id map; live_has(key) is a bit test.

Where blocks * words * 2 passes LIVE_DENSE_MAX (4,194,304 words, 32 MB) the function keeps the
sparse maps (liveness_sparse), so a huge function cannot blow up memory.

Output is unchanged: the sets hold the same leaves and every reader iterates them in the same
sorted order.

## Results

Self build (`ab-host.sh` on linux/arm64 host 2, 7 rounds, A c80e88c3d against B ad63c4965; the
host was loaded, median load 6.2/7.0, but the same slot ratios agree):

| metric | A | B | B/A | 95% interval |
|---|---|---|---|---|
| wall | 27.39 s | 26.38 s | 0.963 | 0.954 to 0.967 |
| CPU | 189.7 s | 178.1 s | 0.939 | 0.928 to 0.939 |
| max RSS | 3.70 GB | 3.54 GB | 0.958 | 0.949 to 0.965 |

The fixed point holds (gate on linux/arm64 host 3: fp, the arm64 suite, codegen patterns, cross,
xounit, the self and seed builds), the self build's output is byte-identical to main's (Mac), and
so is the gcp aiplatform/apiv1 test binary (Mac, cold, sha256 f13df7f1... both).

## Consequences

- insert_rc's per function memory is bounded by the dense cap; above it the old maps run.
- A new reader of live sets goes through live_has or the sorted list, never a key map.

## Alternatives

- Sorted lists of ids per block (no bit sets): smaller for sparse functions, but the join is a
  merge per edge; the cap's fallback covers the sizes where bit sets lose.
- Per function caching of liveness: the rc cache already skips unchanged functions.
