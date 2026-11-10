# A 0588 merge push is refused as a revert

Status: Observed (2026-11-10, Lang 3, std/base64 and std/protoparse)

## Symptom

pushq refuses a decision 0588 stage 2 merge: "the push undoes other
commits' changes", listing `uncovered std/native/M.xo (SHA)` and
`uncovered std/rt/std_gen_M.go (SHA)`.

## Cause

The merge deletes std/native/M.xo and the module's generated Go twins.
pushq's revert check reads a deleted file as setting it back to its text
before the commit that made or last changed it.

## Fix

The deletion is the merge, so name each file in a commit message trailer
(a paragraph of its own at the end) and push again:

```
Revert-ok: std/native/M.xo merged into std/M.xo by decision 0588 stage 2
Revert-ok: std/rt/std_gen_M.go generated file of a split module, removed with the 0588 merge
```

Each reason needs 20 or more characters. Any other file pushq lists is a
real stale tree: rebase instead.
