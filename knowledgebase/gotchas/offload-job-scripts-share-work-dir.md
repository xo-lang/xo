# Two offload jobs that use the same /work directory delete each other's tree

Status: Observed (2026-10-31)

Symptom: a host job fails at once with `tail: cannot open '/work/park-build.log'`
and exit 1, though the same script passed before.

Cause: offload.sh runs up to two jobs per agent at the same time on the
host. A job script that builds in a fixed path (`/work/park`, `/work/park-*`)
and removes it in an EXIT trap deletes the other job's worktree and log when
the first one ends (or recreates it with `git worktree add` under the
other's feet).

Fix: give each job script a directory of its own (`W=/work/NAME`, trap
removing only `/work/NAME` and `/work/NAME-*`), or run one job at a time.
