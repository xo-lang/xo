# tier.sh's 30 minute gocache trim deleted files a concurrent go build was reading

Status: Observed (2026-11-11)

## Symptom

The 04:09 main check (hourly-short) failed TestBuildDevNull:
`xo build --backend=go -o /dev/null` printed `could not import os (open
~/Library/Caches/xo/gocache/ff/ffa07a3d...-d: no such file or
directory)` and "go build failed (a code generator bug)"; the rerun
passed. In the cache afterwards every top-level file (README, trim.txt,
xo-cache.lock, xo-gate.lock) had been created at 03:39:29, `ff/` held 60
files none older than 03:39:30, while the 2-hex directories dated from
two days before: nearly every file had been deleted at once, directories
kept, a few recently used files spared.

## Cause

Not xo's prune (compiler/prune; it re-checks each file's age just before
removing it, keeps files used within 75 minutes, never touches the top
level) and not diskcheck.sh (930 GB free; it trims only under 50 GB).
tier.sh's mac_prep ran, before each Mac step and whatever the free space,

    find ~/Library/Caches/xo/gocache -mindepth 1 -type f -mmin +30 -delete

when tier 908b started at 03:39. Go's cache refreshes a used file's time
only once it is an hour old (cmd/go/internal/cache markUsed,
mtimeInterval), so a file a go build had just looked up could be 59
minutes old: idle "over 30 minutes", it was deleted between the lookup
and the compiler's open. `-mindepth 1` also removed the top-level lock
files the go commands coordinate through (gotool.HoldCache) and Go's
trim.txt.

## Fix

tier.sh's gocache_trim (2026-11-11): only when the disk is under
TIER_GOCACHE_MIN_GB (default 60), only files idle over 2 hours, and only
below the 2-hex shards (`-mindepth 2`). Any trim of a Go cache shared with
running builds must wait out Go's hour plus the longest build (xo's prune:
75 minutes; diskcheck: 2 hours) and must not touch the top level.
