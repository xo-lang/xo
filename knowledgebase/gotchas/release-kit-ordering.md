# Release kit: cold exports reach the tests, and a row pushed after the version commit

Status: Observed (2026-11-12, Xo Lang 2, the 0.50.0 seed and 0.50.1 kits)

## Symptom

1. The 0.50.0 seed cut failed its step 2 (`go test -short ./cmd/xo`):
   `TestLSPModuleCache: after the first check the cache holds 0 module
   entries, want 1`.
2. 0.50.1's browser-memory row (decision 0615) was pushed as a second
   commit after the version commit. pushq rebased it onto a compiler
   commit that had landed between them (6ccc9b351), so the commit to tag
   carried ungated compiler code, and both release gates ran again.

## Cause

1. 0f8cee2ad made seed-cut.sh export `XO_BACKEND_CACHE=off XO_FE_CACHE=off
   XO_CHK_CACHE=off XO_BIN_CACHE=off` for cold seed builds. The exports
   reached every later command, including the tests, and the LSP test
   checks the module cache itself.
2. release-publish.sh refuses a tag whose tree has no ok row for it, so
   the row has to be in the tagged commit. Main moves between two pushes.

## Fix

1. seed-cut.sh runs its tests under `env -u` for the four variables
   (2644ae262). release.sh runs no tests, and its cold exports are for
   builds only. A new test step in either script needs the same `env -u`.
2. Record the row before pushing the version commit, in its own tree:
   build bin/xo, run `scripts/browser-memory.sh --record vX.Y.Z SITE_REPO`
   (BROWSER_MEMORY_SITE_REV for an unpushed site branch), and amend the
   row into the version commit. The gates run on that commit and pushq
   pushes it alone. The tag then names the gated commit.
