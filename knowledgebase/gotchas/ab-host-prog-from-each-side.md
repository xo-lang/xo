# ab-host builds the program from each side's own tree

Status: Observed (2026-11-19, xo-lang-2).

Symptom: an `ab-host.sh BASE NEW NAME -- PROG ARGS` run reports a large
"worse" (B/A 6.35 for bench/perf/brotli enc1 8) when only NEW's commit
changed the benchmark program, here to add a repeat argument: A ran the
program once (31.8 ms), B eight times (202 ms).

Cause: scripts/lead/ab-host-remote.sh builds PROG from each worktree,
`cd "$W/$s/$prog"` for s in a and b, so BASE's copy of the program runs
on A's side with the same arguments.

Fix: a change to the benchmark program for an --ab goes on both sides.
Commit it on BASE too (`git worktree add -b NAME DIR BASE`, then
`git cherry-pick` the bench commit) and pass that commit as BASE. The
local commits need no push: ab-host bundles them. Check that the A and
B medians are in the range one run of each gives before reading B/A.
