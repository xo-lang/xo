# A warm-up build races std's detached variant build

Status: Observed 2026-11-08 (xo repl warm-up, decision 0572).

## Symptom

The REPL's first warm-up was a plain `xo build` of a sample session
program. On the Mac it looked fine (first entry 236 to 269 ms, against
1.2 s with no warm-up). On linux/arm64 host 1 the first entry still took
0.26 to 0.73 s, the times rose session after session (the no warm-up row
went 1.25, 2.26, 2.83, 3.85, 6.01 s), and `rm -rf` of a session's cache
said "Directory not empty".

## Cause

A fresh xo misses std's concurrency variants, so std_variant_peek
(compiler/xo/stdcache.xo) builds the whole program and starts the
variants' build in a detached, niced `xo dev stdcache` child, followed by a
warming rerun of the same build. Waiting for the warm-up's `xo build` does
not wait for that child: the first entry raced it (the entry built whole
program again, or competed for CPU), and children from earlier sessions
were still writing into caches being measured and deleted.

## Fix

The warm-up builds the variants itself, in the foreground
(`xo dev stdcache --conc=0`, then `--conc=1`), then builds the sample with
`XO_SEPCOMP_BG=0`, so nothing outlives it (compiler/cli/repl warm_start).
First entry: 71 to 99 ms on the Mac, 80 to 127 ms on host 1. Anything that
wants caches warm before a later build must do the same: a build on a
variant miss returns before the variants exist. When measuring repeated
fresh-cache builds, also wait for background children (the cache's size
settling) or set XO_SEPCOMP_BG=0 in the control.
