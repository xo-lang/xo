# A run cache hit wrote the binary and no dSYM

Status: Observed 2026-11-18 (task #359, objverify.sh).

## Symptom

`xo build --dsym -o a/prog .` then `xo build --dsym -o b/prog .` left
`b/prog` with no `b/prog.dSYM`; `dwarfdump --verify b/prog.dSYM` found no
file. With `--phase-times` (another run key) the dSYM was there, so it
looked intermittent.

## Cause

The whole-build run cache (decision 0360) keys a build independently of
its `-o` directory since 2026-11-15, so the second build was a hit. A hit
writes the stored binary and the entry's side files, and the only side
compiler/xo/main.xo recorded was a windows `.pdb`. The dSYM bundle and a
split build's `.debug` were never stored.

## Fix

main.xo records `.dSYM/Contents/Resources/DWARF/PROG` and `.debug` as
sides when this build wrote them (a stale one stores no entry, as for a
PDB), and wholebuild.xo's hit creates the bundle's directories before
writing a side. Test: cmd/xo TestWholeBuildDebugSides (fails before).
Any new file a build writes beside its binary must be a side too, or a
hit drops it.
