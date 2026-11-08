# Seed cut: three environment failures in one cut

Status: Observed (2026-11-08, the 0.44.0 seed cut; Xo Lang 2)

## Symptom

The 0.44.0 cut (`scripts/seed-cut.sh`) failed three times before a
fourth try ran through, and the fixed point never failed:

1. `seed-cut: Docker is not running`, right after a helper had seen the
   daemon answer: Docker Desktop had just started and stopped answering
   for a few seconds.
2. Step 6, the cross builds: `curl: (22) The requested URL returned
   error: 504` and `race-runtime.sh linux-arm64 failed`, about seven
   minutes in. race-runtime.sh downloaded llvm-project's source (about
   150 MB, from GitHub's release assets) into each platform's WORKDIR,
   on every cut, with no retry.
3. Step 1: `fatal: '.../seed-0.44.0/tree' is a missing but already
   registered worktree`. The WORKDIR from try 2 had been deleted, but
   git still listed its worktree.

## Cause

seed-cut.sh checked Docker once, and never checked for a registered
worktree under its WORKDIR. race-runtime.sh fetched the pinned source
fresh per WORKDIR, so every platform of every cut depended on GitHub
answering.

## Fix

- race-runtime.sh keeps a verified copy of the pinned source in a cache
  that every WORKDIR and cut shares (`$XO_RACE_SRC_CACHE`, else the
  user cache's `xo/race-src`). It downloads only on a miss, with
  `curl --retry 5`, which retries a 5xx. The Docker builds get the file
  copied into their WORKDIR by the host, and each copy is checked
  against the pinned SHA-256.
- seed-cut.sh gives Docker a minute of retries. Before step 1 it fails
  in seconds when a worktree under the empty WORKDIR is still
  registered, and it prints the `git worktree remove --force` to run.
  It does not prune: `git worktree prune` would also drop other
  sessions' worktrees whose directories are gone.
- After a failed cut, remove its worktree (`git worktree remove
  --force WORKDIR/tree`) together with the WORKDIR.

## A cut from a tag older than this fix

The fourth try hit the 504 again, in step 5's darwin race runtime.
seed-cut.sh runs race-runtime.sh from the tag's tree, so a tag cut
before this fix still downloads. The fifth try worked around it as
follows:

1. Download the source once with `curl --retry 10`, check its SHA-256,
   and put it in the cache.
2. Once seed-cut.sh had made its WORKDIR (its `logs/` directory
   exists), copy the file into `race/dl`, `race-darwin-amd64/dl` and
   `race-{linux-arm64,linux-amd64,windows-amd64}/w/dl`.

The old script skips the download when the file is present, and it
still checks the SHA-256.
