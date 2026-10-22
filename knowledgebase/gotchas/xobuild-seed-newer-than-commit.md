# A seed newer than the commit builds an xo that cannot start

Status: Observed (2026-10-22), fixed the same day.

## Symptom

Building an older commit's xo to bisect a failure on main
(`go run ./scripts/xobuild` in a worktree at f3da86890, 2026-10-21
05:46) gave a bin/xo that died at once:

    dyld: Symbol not found: _xo_x_preempt
      Referenced from: .../wt-bis/bin/xo

With the worktree's newer bin/xo left in place, the build used that as
its seed and the result died with an abort trap instead. Bisecting
across the 2026-10-21 runtime changes was impossible.

## Cause

xobuild's seed order took the first xo that built compiler/xo: the
checkout's bin/xo (built from a newer commit), then the newest prebuilt
seed (0.24.0, cut from edf36b43 at 14:25 that day). A seed compiles the
checkout's compiler against the checkout's std, but its code generator
is its own: 0.24.0 emits loop polls that call `xo_x_preempt` (decision
0286), which the older std/native runtime does not define. The link
leaves it to the dynamic loader, so the build "succeeds" and only
running the xo fails. Era mixing, in other words: a newer compiler with
an older runtime.

## Fix

internal/xotool (2026-10-22):

- FetchSeedFor: when the newest seed's commit is in the repository but
  not an ancestor of HEAD, use the newest seed (unpacked in the cache, or
  listed at the seed location) whose commit is (the seed name's
  `<version>-<8 hex>`). f3da86890 now builds with seed 0.23.0.
- Seeds: the checkout's bin/xo is skipped when `bin/xo.commit` names a
  commit that is not an ancestor of HEAD.
- BuildXo runs the new xo's `xo version` before accepting it, so an xo
  that cannot start fails that seed and the next one is tried.

Tests: TestFetchSeedForAncestor; TestBuildOldCommit
(`XO_TEST_OLD_BUILD=1`, several minutes) builds f3da86890's xo with a
private checkout and back-end cache and runs a program with it.

A bisect still needs a private `XO_BACKEND_CACHE_DIR` for the old xo
(gotchas on old prunes), and an older commit than the oldest seed
(0.9.0) still needs `XO_SEED_FALLBACK=chain`.
