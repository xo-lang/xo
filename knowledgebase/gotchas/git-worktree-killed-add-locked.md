# A killed `git worktree add` leaves a locked registration that prune keeps

Status: Observed (2026-11-19 and 2026-11-20, gatesteps-canary on host 4; #453, #501)

Symptom: a script that reuses a fixed worktree path fails on its next run with
"fatal: 'W' is a missing but already registered worktree" or "is a missing
but locked worktree", although it runs `git worktree prune` first.

Cause: prune keeps two kinds of entry. A worktree whose directory is still on
disk is live, so `rm -rf W` has to come before the prune (#453). A run
killed inside `git worktree add` leaves the registration locked
("initializing") with no directory, and prune never removes locked entries
(#501).

Fix: `rm -rf "$W"; git -C "$P" worktree prune`, then `git -C "$P" worktree
add -f -f ... "$W"`; the doubled -f overrides a missing locked registration
(scripts/lead/gatesteps-canary-remote.sh; tested in gatesteps_canary_test.sh).
