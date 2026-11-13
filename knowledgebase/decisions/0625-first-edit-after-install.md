# 0625: A fresh install's first edit reuses its program's caches

Status: Proposed (Xo Lang 2, 2026-11-13; stub, being filled)

## Context

release-vs-go v0.50.1 (bench/results/release-vs-go-v0.50.1.md, armcompute row): a fresh install's
first program builds whole (std's variants are not built yet), and its first edit after that took
471 ms against Go's 304, where an edit with the variants already built takes 226 ms. Under study:
whether the background std build's warm step (XO_STD_BG_WARM_OUT, stdcache.xo) already closes this
for a user (the timing ran under heavy.sh, which sets XO_SEPCOMP_BG=0), and if not, a background fill
of the program's separate-build caches after a run-cache hit.

## Decision

To be written after the measurements.
