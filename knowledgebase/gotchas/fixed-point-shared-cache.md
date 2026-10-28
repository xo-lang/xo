# A determinism check that shares a cache tests the cache too

Status: Observed (2026-10-27)

## Symptom

`scripts/bootstrap-xo.sh` (the fixed point: s2 and s3 must write the same
assembly and binary) failed once on a branch whose change touched no code
generation, under a heavily loaded machine (several agents gating):
"s2 and s3 assembly differ", "s2 and s3 binaries differ". The rerun on the
same source passed, and ten runs on main passed. The run's outputs were
deleted with the rest, so the differing function is not known.

## Cause

The script gave each stage an empty build directory, runtime object cache
and binary cache, as its header said ("nothing is reused from an earlier
build"), but not the back-end cache: `XO_BACKEND_CACHE_DIR` was unset, so
s1, s2 and s3 read and wrote the user's `<user cache>/xo/backend`, shared
with every xo on the machine (other worktrees' builds, older xos) and
pruned by all of them. s3 took s2's objects instead of compiling them (an
s3 of 10 s, an s2 of 14 s in one run), so the check compared the cache with
itself as much as the compiler with itself, and an entry another xo wrote,
or one read while another process pruned or replaced it, could make s2 and
s3 differ (or hide a real difference). The cause of the one failure is not
proven: XO_BACKEND_CACHE=verify runs of the shared cache are the test for
a key that misses an input.

## Fix

Every stage and seed chain hop builds with its own back-end cache under
OUTDIR (`XO_BACKEND_CACHE_DIR=$out/cache-sN/backend`). A determinism check
(fixed point, identity, reproducible build) isolates every cache the
compiler reads, not only the ones it names; keep a failing run's outputs
until its first differing function is found.
