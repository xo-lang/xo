# Every xobuild after a compiler edit rebuilt cold

Status: Observed 2026-11-05 (fixed the same day, bench/results/edit-loop-2026-11-05.md).

## Symptom

A one-line edit under compiler/ssa, rebuilt with `go run ./scripts/xobuild`,
took 49 to 65 s on the Mac and about 52 s on linux/arm64, every time, with
`XO_CACHE_STATS=1` reporting `fe 0/12122 bodies`, `chk 0/23 modules` and
almost no reused functions or objects. The same sources built a second time by
one fixed xo took 5 s.

## Cause

Three things, each enough to empty the caches:

1. The seed of edit N was the checkout's bin/xo, the xo of edit N-1. A
   compiler/ssa edit changes the codegen key (`XO_CODEGEN_KEY`), the
   identity the front end and backend caches are keyed on
   (compiler/xo/bcache.xo `cache_id`), so that xo found nothing written under
   its key.
2. xobuild stamps the build (`XO_DIRTY`, `XO_SOURCE_KEY`, `XO_CODEGEN_KEY`),
   and the stamps change with every edit. `stamp_root(root, stamps)` was in
   every program body's front end key, though stamps only replace constants of
   the root module (compiler/xo/stamp.xo): 3,914 of 12,122 bodies hit.
3. When the nearest cached clean build was more than `HopMax` (50) commits
   behind HEAD, xotool's `SecondHop` built again with the xo just built from
   the edited sources: a new builder, and a cold build, for every edit.

## Fix

- `Seeds()` tries the newest clean build of an ancestor commit before the
  checkout's earlier bin/xo: a stable builder whose caches stay warm.
- `fe_keys` (compiler/ssa/fecache.xo) uses the stamped root only for the root
  module's bodies.
- With uncommitted changes and no clean build within `HopMax` of HEAD,
  `headBuilder` builds HEAD once in a temporary worktree; that build is the
  stable builder of the edits that follow.
- A stamped build checks its root module in full (Checker.mod_noskip_root):
  the root's bodies still miss on the new stamps, and skipping its check
  made the driver drop the first pass (about 3.5 s) and check again.
- The stable builder skips the precheck and the seed smoke (it passed the
  same smoke when it was built; a source error fails its build as fast).

To check a rebuild, `XO_XOBUILD_TIMES=1 go run ./scripts/xobuild` prints each
step's time and the build's `--phase-times` and `XO_CACHE_STATS`.
