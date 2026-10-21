# Base against new identity runs: one std path, every cache off

Status: Observed (2026-10-21)

## Symptom

1. A byte for byte comparison of linux/arm64 builds made by a base xo
   and a new xo reported every program different (compiler/xo and small
   goldens alike) for a change that could not alter output.
2. An identity sweep over the goldens (base and new xo, darwin and
   linux) filled the disk: free space fell from 95 GB to 47 GB in about
   35 minutes on a machine shared by several agents.

## Cause

1. Each xo finds std next to its binary (bin/../std), so the base xo of
   another worktree built with that worktree's std, and the binaries'
   DWARF names std's files by their path: different paths, different
   bytes, though the code was identical.
2. The sweep left the back-end, front-end, and part caches on: every
   build of every golden stored entries under two compiler keys and two
   targets, never read again.

## Fix

- Build both sides with the same `XO_STD` (the new tree's std) and the
  same source directory; on darwin also the same output name
  (gotchas/darwin-binary-identity-output-name.md).
- Turn every cache off for a sweep (`XO_BACKEND_CACHE=off XO_FE_CACHE=off
  XO_PART_CACHE=off XO_BIN_CACHE=off`), delete each base and new pair
  right after comparing it (keep only mismatches), and remove the sweep's
  worktrees at its end.
