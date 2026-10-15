# git stash is shared by every worktree

Status: Observed (2026-10-15).

## Symptom

In a scratch worktree with no local changes, `git stash -q; git pull
--rebase; git stash pop` applied an unrelated work in progress (another
agent's stash from a different branch) and left conflicts.

## Cause

The stash is a ref (refs/stash) in the shared repository, not per
worktree. With nothing to stash, `git stash` makes no entry, and `pop`
takes whatever entry is on top, whoever made it.

## Fix

Do not stash around a pull: `git pull --rebase --autostash` (it stashes
and restores only when there is something to stash), or commit first.
If someone else's stash was applied, `git reset --hard HEAD` restores the
worktree when all of one's own work is committed; leave the other
entry in the list.
