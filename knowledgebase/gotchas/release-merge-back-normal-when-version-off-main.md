# Merging a release back: -s ours drops a version commit that is only on the release branch

Status: Observed (2026-11-13)

## Symptom

The 0.51.0 cut planned a `-s ours` merge of the release commit into main
(as 0.50.1's respins were merged). For 0.51 that merge would have left main
saying 0.50.1 (compiler/cli/core VERSION, compiler/cli/servers XO_VERSION,
std/VERSION), at spec draft 0.51, and without the release's browser-memory
row, while the tag said 0.51.0.

## Cause

`-s ours` keeps main's tree and records only the ancestry. That is right
when main already holds the release's tree (0.50.1: its version commit
778b69ef0 was on main, and the respins differed only in fixes main had too).
0.51.0's version commit (c78769cbf: version x3, spec drafts, CHANGELOG
section, browser-memory row) was made on the release branch only.

## Fix

When the version commit exists only on the release branch, merge it back
normally (`git merge --no-ff N`): only the release's own files change. The
CHANGELOG conflicts; write it with the release kit's changelog-split.py and
check it with changelog-split-check.py (v0.51.0 is the release's text,
Unreleased keeps only what landed after the cut). Use `-s ours` only when
main already has the release's tree. A merge commit is pushed directly with
a guard on origin/main unchanged, since pushq rebases and would flatten it.
0.51.0: 07d042a45 (parents a64d3a2b9, c78769cbf).
