# A host cleanup removed a running evidence job's worktree

Status: Observed (2026-11-13)

## Symptom

A cleanup of Lang 3's own outputs on the second offload host removed
/work/spl3-jsonstress3, the worktree of a json stress run that was 40
minutes into its 60. Its checks kept nothing: no process line named the
path, and no file under it was newer than 30 minutes.

## Cause

The checks looked for the wrong traces of use. stdproven-remote.sh
checks the worktree out and builds xo in the first minutes; after that
the stress programs run from /tmp/stdproven-stress-*/ binaries and the
`go run ./scripts/stdproven` process names no path, so a 60 minute run
writes nothing under its worktree for most of its life.

## Fix

Remove an evidence worktree only after its own run ended: the local log
of that run (notes/lang3/NAME.log) ends with `rc=`, or the run's wrapper
(`bash /tmp/off.*` with the job NAME in its arguments, which ps shows
for the whole run) is gone. Match by the job NAME, not by the path.
