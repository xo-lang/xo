# A dirty tree's xo offered to another checkout as an ancestor's build

Status: Observed (2026-10-16), fixed the same day.

## Symptom

Another session's `go run ./scripts/xobuild` built its bin/xo with a seed
it found as "the newest earlier build", and that xo died at once:
`dyld: Symbol not found: _xo_x_task_spawn_fn`.

## Cause

internal/xotool caches each checkout's bin/xo in `<user
cache>/xo/checkout/<key>` and writes the HEAD commit beside it (the
`commit` file); ancestorBuild offers the newest cached build whose
commit is an ancestor of the other checkout's HEAD. A build from a
worktree with uncommitted changes (Xo lang 4's spawn change in progress)
was recorded under its HEAD, f28032fd, though its xo came from different
sources: it emitted calls to a runtime export (`task_spawn_fn`) that the
commit's std does not have. Any checkout descending from f28032fd then
got that xo as its seed.

## Fix

xotool.go recordCommit: a build is recorded (the `commit` file and a new
`clean` file) only when `git status --porcelain -- selfhost std` is empty;
a dirty tree's build gets neither. ancestorBuild offers only builds with
the `clean` file, so records written before the fix (the poisoned one
included) are never offered again. Tests: internal/xotool
TestRecordCommitDirty, TestAncestorBuild.

To avoid it when working: build a dirty tree's xo with `XO_CLI` or
`XO_SEED` pointing at a known xo, and never copy one into another
checkout's bin/.
