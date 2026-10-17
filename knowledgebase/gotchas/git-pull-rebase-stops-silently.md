# git pull --rebase that stops on a conflict looks like it succeeded

Status: Observed (2026-10-17, Xo lang 3)

Symptom: a gate's `git pull -q --rebase origin main && ...` chain went
on as if the rebase had worked. The worktree was in the middle of the
rebase (the pick applied but not committed, HEAD on the new base), and
CHANGELOG.md held a conflict's markers. The gate built and tested that
tree. A later `git commit --amend` then amended the base commit instead
of the picked one. It was caught before the push, when the next pull
refused to start ("a rebase-merge directory").

Cause: with -q git prints the conflict to stderr and exits nonzero, but
the agent's chain read only the gate's own log, and an interrupted rebase
leaves a clean looking `git status --short` (the conflicted file can be
staged in the pick).

Fix: after every `git pull --rebase` (and `git rebase`), check `git
status` for "rebase in progress" before gating or amending, and look at
the exit code. internal/repotest TestNoConflictMarkers (-short) fails on
conflict markers in any tracked text file, so a half finished rebase
cannot pass -short. To repair one like this: `git reset --soft <base>`,
commit the change with its message (`git commit -C <old sha>`), `git
rebase --quit`, and point the branch at the new commit.
