# Evidence rounds on one host share a worktree, and leftover evidence blocks its checkout

Status: Observed (2026-11-16 and 2026-11-17, xw-1, task #152)

- Symptom 1: two std Proven evidence rounds (stress, race) started for
  the same offload host at once, each through `stdproven-offload.sh`
  on that host's one worktree. The second round's `git checkout
  --detach origin/main` moved the tree under the first while it ran,
  and both wrote one log, so neither result could be read.
- Symptom 2 (2026-11-17 00:23): a round's checkout of origin/main failed
  with "untracked working tree files would be overwritten by checkout".
  The untracked evidence files a finished round had left for its push
  (`std/MODULE/evidence/...`) had meanwhile landed on main from another
  push of the same module's evidence.
- Cause: a host worktree and a round's log name are per host, not per
  round; evidence stays untracked between a round and its push, so a
  later checkout meets the same paths arriving from main.
- Fix: one round per host at a time (a `mkdir` lock per host, its PID
  inside, a stale lock taken over when that PID is gone); a log name per
  round (`KIND-hHOST-$$.log`). Push or remove a round's evidence before
  the next checkout; when the checkout fails on evidence paths, compare
  the files with main's (`git diff --no-index`), delete the untracked
  copies that main already has, and rerun the round.
