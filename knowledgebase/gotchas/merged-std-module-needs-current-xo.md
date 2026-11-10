# A merged std module fails under an xo older than its merge

Status: Observed (2026-11-10, decision 0588).

## Symptom

A pushq gate on a rebased worktree failed every native golden while
building std's conc=0 variant: `xo dev stdobj: std/uuid.xo:119:24: error
XO1201: not available on the native backends ... this call`. The log
line before it said `xotool: xo .../bin/xo (key ..., commit 93d9b36d0081);
tree key ... (differs)`.

## Cause

The worktree's bin/xo was built before 34d81e1da (0588 stage 1), and the
rebase brought in a std where uuid is merged (std/merged.txt). An xo
without the stage 1 loader parses std/uuid.xo as a declaration file and
looks for std/native/uuid.xo (gone), so the first call to a private
helper has no call information. Main was fine: its own xo built both std
variants.

## Fix

Rebuild bin/xo after a rebase that brings in a merged module (`go run
./scripts/xobuild`), as for any std change (a `tree key ... (differs)`
line means the xo is stale). Only an xo from before stage 1 is affected:
later builds read std/merged.txt when they load std, so a module merged
after them loads too.
