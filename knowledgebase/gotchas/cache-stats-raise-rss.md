# XO_CACHE_STATS raises a small build's peak RSS

Status: Observed (2026-11-01, decision 0448)

## Symptom

A breakdown of coll's warm build read 115 MB of RSS where `rss-guard.sh` read 88 MB on
nearly the same commit, with the same recipe.

## Cause

The breakdown set `XO_CACHE_STATS=1`, as `scripts/release-vs-go.sh` did for its timed
builds; the guard does not. Warm coll at c101874ea, the same caches, 3 alternating rounds:

| XO_CACHE_STATS | Max RSS | Peak footprint | Instructions |
|---|---|---|---|
| unset | 89, 88, 86 MB | 52 MB | about 1340 M |
| 1 | 116, 116, 117 MB | 70 to 72 MB | about 1375 M |

Why the statistics cost this much has not been found. They wrap the build's Fs
(compiler/xo/cachestats.xo), though the link's store read already bypasses the wrapper
(`ctx.real_fs`).

## Fix

Measure memory without the statistics. `release-vs-go.sh` now times its builds without
them and takes the edit's path from an untimed build of another edit. Its reports through
v0.35.0 include the cost in xo's peak MB, so comparing across that point shows a step with
no change in the compiler. Their wall and instruction figures carry the smaller cost too.
