# Flag values in the command line made every self-build a new pack target

Status: Observed (2026-10-22, Xo Publish).

## Symptom

The back-end cache's prune ran every minute on the Mac and removed 26 to
90 GiB and 1.0 to 2.6 million files an hour (prune-log, 2026-10-21 19:00
to 2026-10-22 01:00) to hold the 16 GiB cap, walking the cache for 10 to
18 minutes of every hour. 56 part pack segments (8.3 GB, half the cap)
were in the cache, none read after it was written, and every pack
segment was under 30 minutes old. Each `go run ./scripts/xobuild` of a
new tree key wrote one more 678 MB segment, in a new slot.

## Cause

compiler/xo bcache.xo pack_dir names a build target by the working
directory and the command line's words without the flags. It skipped
`-o`'s value only. scripts/xobuild passes `--stamp XO_COMMIT=...
--stamp XO_SOURCE_KEY=... --stamp XO_CODEGEN_KEY=...`: the values are
words, so each tree key was a new target. A module of 16 parts or more
packs its parts on a target's first build (1974c0f11), so each self-build
wrote xo's whole parts pack (116 parts), and the next self-build, a new
target again, regenerated every part instead of reading it.

## Fix

pack_words (bcache.xo) leaves out the value of every flag of
value_flags() written as a separate argument. A second stamped self-build
reuses 115 of 116 parts (9.5 s against 11.0 s) and the packs hold 666
against 1,295 MB. The fix takes effect for builds made by an xo that has
it (xobuild builds with the earlier xo).
Remaining: a worktree's first self-build still writes the parts pack,
and the loose entries still turn over every 20 to 30 minutes at the cap
(measure again after this lands).
