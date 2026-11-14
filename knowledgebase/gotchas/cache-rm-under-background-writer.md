# Emptying a cache directory between cold rounds fails while the background std build still writes

Status: Observed (2026-11-13)

## Symptom

A cold, default-cache `xo bench --ab` (decision 0629's headline row on
linux/arm64 host 1) emptied its cache directories before every run with
`--guard="rm -rf DIR/backend DIR/bin; mkdir -p ..."`. Several rounds printed
`rm: cannot remove '.../backend/std/.build-.../cache/hfacts': Directory not
empty`, so those rounds started with part of the cache left: partly warm
on both sides, and a weaker headline. The wall ratios were also noisy.

## Cause

A cold build with the default caches starts a detached background child
(`XO_STD_BG_CHILD=1`, compiler/xo/stdcache.xo) that builds std's object into
`CACHE/std/.build-ID/` and renames it into place. The child holds
`CACHE/std/.bg.lock` and outlives the run that started it. The guard's
`rm -rf` raced the child's writes, so files appeared under directories
`rm` was removing. The child also used CPU during the next measured round.

## Fix

Give every round a fresh, uniquely named cache directory instead of
emptying a reused one. Point the cache at a symlink (`XO_BACKEND_CACHE_DIR=D/cur/backend`,
`XO_BIN_CACHE=D/cur/bin`). Make the guard a script that first exits 1
while the current cache's `std/.bg.lock` is held (`flock -n LOCK true`
fails; `xo bench --guard` retries until it exits 0). Then it points `cur`
at a new `mktemp -d D/r.XXXXXX` and removes the old round's directory, which
nothing writes any more. The child names its directories from the path it
was given, so switching the symlink while it runs would send its renames
into the next round's cache. Wait for the lock before switching.

Example: ~/xo-lead/notes/lang1/aipab-dc2-remote.sh (2026-11-13).
