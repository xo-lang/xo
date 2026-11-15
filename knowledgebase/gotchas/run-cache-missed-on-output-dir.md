# The run cache missed every build into a new output directory

Status: Observed (2026-11-15, Xo Lang 2, measuring decision 0657's builds)

## Symptom

A warm `xo build -o DIR/prog .` of an unchanged program took 230 to 312
ms on the Mac into a new DIR and 13 ms into the same one; on host 3 the
warm head-side builds of 743 goldens took 124 s against 158 s cold (21%
saved, not 95%). Golden tests, which build into a new temporary
directory, never hit.

## Cause

Two things held the output's directory. The key held the full command
line, `-o DIR/prog` included. And the entry listed the output directory
as an input (`dir DIR` in the .m file: the build lists it), so even a
key without DIR missed until the listing was dropped. A first fix of the
key alone found the entry but the hit failed: the new DIR did not exist
and the hit copied without making it, then built anyway.

## Fix

compiler/xo out_dir_free (decision 0360 addendum 2026-11-15): the key
keeps -o's file name, the entry drops the output directory's listing and
probes unless the build read a file there, and a hit makes the directory.
Check a run cache change with `--phase-times`: `phase run_cache` followed
by `phase run_cache_miss` means an entry was found and the hit failed.

## Addendum 2026-11-15: XO_BUILD_DIR and the replayed output

Two more held the output's place after the fix above. Tests give each
build its own XO_BUILD_DIR (xotool XO_TEST_BUILD_SCRATCH), and the key
and the entry's env lines held it ("environment changed: XO_BUILD_DIR");
it is now out of both, as the build directory is scratch. And a hit
replayed what the stored build printed, its own output path
(`darwin/arm64: OLD/prog`), so `XO_BACKEND_CACHE=verify` reported XO1512
"what the build printed" for every hit into a new directory, live on main
since 530cddfba; a hit now prints its own cleaned path (wholebuild.xo
hit_output), and verify compares that. Check: 60 goldens under verify
on the Mac, 59 entries found and verified, 0 XO1512, binaries the same
(native_sqlite never hits: its entry lists the temporary dSYM it wrote).
