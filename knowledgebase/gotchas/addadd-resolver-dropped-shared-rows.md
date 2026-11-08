# The add/add resolver dropped the field lines two std/changes entries share

Status: Observed 2026-11-08 (Xo Lang 1)

## Symptom

A std/changes entry pushed through pushq (or rebased with
`~/xo-lead/agents/resolve-addadd`) lost its `- Commit: this push` line,
and a resolved entry could lose `- Shipped in: not yet released` too.
Nothing failed: the entry still had its heading, Kind and prose. Five
entries on main lacked their Commit line (crypto.md's SHA-512 blocks and
Ed25519, prelude.md's scavenge fix, 16 KiB free blocks and one byte
find).

## Cause

For an add/add hunk the resolver (scripts/lead/resolve) writes ours, then
theirs without the `- ` and `| ` rows ours also added. The rule was for
index lines both sides bring in (decisions/README.md's 0530 and 0531 on
2026-11-06). Two new std/changes entries in one hunk both have
`- Kind: perf`, `- Commit: this push` and `- Shipped in: ...`, so the
second entry lost the ones it shared with the first.

## Fix

c7a4435cc: theirs' shared rows are dropped only when theirs is all list
or table rows (an index hunk); a block with a heading or prose keeps
every line (test "add/add keeps the field lines two added entries
share"). 1367b5caf restored the five Commit lines with their hashes.
After a rebase that resolved a std/changes or CHANGELOG conflict, read
the new entry once (`git show HEAD -- std/changes`) before pushing.
