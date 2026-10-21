# A pinned XO_CLI goes stale when pushq rebases its worktree

Status: Observed (2026-10-21, xo-fa)

Symptom: a deferred check run after a push failed seven internal/llvm
tests with errors that made no sense for the change: `function embed not
found` in the embed_files and embed_dir goldens, a missing traceback
local, HTTP parse differences, and preemption timeouts.

Cause: the check pinned `XO_CLI` to the pushed worktree's `bin/xo`,
built at the gate. pushq rebases a worktree onto origin/main before
pushing, so the tree's goldens and std were newer than that xo (here they
included decision 0304's embed).

Fix: run post-push checks in a fresh worktree at origin/main with its
own `go run ./scripts/xobuild`, or pin a copy of the xo and its std. A
worktree pushq has pushed is newer than its `bin/xo`.
