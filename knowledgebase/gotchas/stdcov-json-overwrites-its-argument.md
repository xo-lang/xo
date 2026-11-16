# stdcov -json writes its argument, it does not read it

Status: Observed (2026-11-16)

## Symptom

After `go run ./scripts/stdcov -json std/protodesc.xo -o /dev/null`, std/protodesc.xo held a JSON
document (`{"sha": ..., "modules": [...]}`) instead of Xo source, and a commit made right after carried
it; the module's gate then failed to build.

## Cause

`-json FILE` is an output flag: stdcov writes every module's marks to FILE (the snapshots xw std reads).
It takes no module or source argument, so passing a source file overwrites that file.

## Fix

Name a scratch path outside the worktree: `go run ./scripts/stdcov -json ~/xo-lead/notes/NAME/marks.json -o /dev/null`,
then read the module's row there. Restore a clobbered file with `git checkout BASE -- FILE` and check
`git diff --stat` before committing.
