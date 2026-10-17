# grep -q under pipefail fails on long input

Status: Observed (2026-10-14).

Symptom: scripts/ci/suite.sh `auto` printed "fixed point: not needed for
these paths" (and skipped the internal/selfhost merge tier) for a change
that touched compiler/. A reproduction with 1,000 or more changed paths
skipped it every time; 400 paths never did.

Cause: `cmd | grep -q PATTERN` under `set -o pipefail`. `grep -q` exits at
its first match and closes the pipe; when the writer still has output
left (more than a pipe buffer, so long lists), its next write gets
SIGPIPE and it exits 141. pipefail makes the pipeline's status that 141,
so the `if` sees false although grep matched. Short lists fit the pipe
buffer and pass, so it only fails on large changes.

Fix: do not pipe into `grep -q` under pipefail. List once into a
variable (`files=$(git diff --name-only ...)`) and test it with a here
string (`grep -qE PATTERN <<<"$files"`), or let the last grep read all
its input (`grep PATTERN >/dev/null`).
