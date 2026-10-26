# A darwin release build carries its file name in the code signature

Kind: observed (2026-10-25, xo-deepseek-2, comparing two Map shape binaries).

## Symptom

Building the same program from the same tree twice gives two binaries with
different SHA-256 hashes. `cmp -l` reports one or a few bytes, near the end
of the file, inside the embedded signature. The sizes differ by exactly the
difference in the lengths of the two output names. `__text` and the symbol
counts are identical in every pair.

## Cause

The linker writes an ad-hoc signature whose identifier is the output
FILE NAME, not its path: `codesign -dv det-a` prints `Identifier=det-a`
and `codesign -dv det-b` prints `Identifier=det-b`. The directory does not
leak in, and nothing else varies between builds:

- two builds of the same program to `det-a` and `det-b` differ in exactly
  one byte, the `a` against the `b`;
- two builds to `/tmp/sb4/sign-x/same` and `/tmp/sb4/sign-y/same` (the
  same name in different directories) are byte identical;
- a rebuild of the measured tree to the measured binary's own path
  reproduces that binary byte for byte.

## Fix

To check that a tree still produces the binary that was measured, build to
the same output FILE NAME (copy the measured binary aside first, or build
to another directory with the same name) and then compare. Do not read a
hash mismatch between differently named outputs as "the code changed", and
do not conclude the build is unreproducible from it: compare `__text` and
the symbol counts first, and pin the differing bytes before writing it up.
