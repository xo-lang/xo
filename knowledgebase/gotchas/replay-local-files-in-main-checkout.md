# Recorded replay fails in the main checkout but passes in worktrees

Status: Observed (2026-10-13, merging decision 0215)

## Symptom

After the Go compiler was deleted, internal/selfhost replays recorded
outputs (XO_SELFHOST_GOLDENS=replay). Its -short run passed in the
deletion agent's worktree and failed in the main checkout:

- TestSelfhostDifferential, ParserDifferential, Format: "no recording for
  lexdump runs/codex-xo-r2/... not even of an earlier input";
- TestSelfhostChecker: "5 of 748 runs differ" (gocalls, stage6c,
  stage6c_skipped, examples/realtime-control).

## Cause

Two kinds of local files exist only in the long lived main checkout:

- the gitignored runs/ tree (benchmark run history), which the repo file
  walk (repoXoFiles) picked up; nothing records those files;
- stale generated binding caches, `<module>/.xo/gobind` and `.xo/cbind`
  (gitignored), left by runs on 2026-10-10 and 2026-10-11, which the
  checker reads when present, so its output differs from the recording
  made in a clean tree.

The live Go comparison before 0215 compared both sides on the same files,
so neither showed up.

## Fix

- repoXoFiles lists files through `git ls-files --cached --others
  --exclude-standard`, so ignored trees never reach the replay.
- Stale `.xo/` caches in the main checkout were moved aside (18
  directories). If a replay fails only in one checkout, look for ignored
  `.xo/` directories first: `git status --short --ignored | grep '/\.xo/$'`.
- Open: the checker trusting a stale `.xo/` binding cache is a product
  question as well (a user upgrading xo keeps old caches); the caches
  should be keyed by the generator's version.
